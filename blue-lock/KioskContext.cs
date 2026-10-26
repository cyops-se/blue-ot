using Microsoft.Win32;
using static BlueLock.NativeMethods;
using Timer = System.Windows.Forms.Timer;

namespace BlueLock;

/// <summary>Owns the kiosk windows, the slideshow and the lockdown for the lifetime of the app.</summary>
internal sealed class KioskContext : ApplicationContext
{
    private static readonly HashSet<string> ImageExtensions =
        new([".jpg", ".jpeg", ".png", ".bmp", ".gif", ".tif", ".tiff"], StringComparer.OrdinalIgnoreCase);

    private const int ExifOrientation = 0x0112;

    private readonly KioskSettings _settings;
    private readonly SynchronizationContext _sync;
    private readonly KeyboardHook _hook = new();
    private readonly KeySequenceDetector _exitDetector;
    private readonly LockdownPolicies? _policies;
    private readonly Timer _slideTimer;
    private readonly Timer _guardTimer;
    private readonly List<SlideForm> _forms = [];
    private readonly uint _processId = (uint)Environment.ProcessId;
    private readonly DateTime _startedUtc = DateTime.UtcNow;

    private List<string> _files = [];
    private int _index;
    private Image? _current;
    private bool _shuttingDown;

    public KioskContext(KioskSettings settings)
    {
        _settings = settings;
        _sync = SynchronizationContext.Current ?? new WindowsFormsSynchronizationContext();
        _exitDetector = new KeySequenceDetector(settings.ExitKeys, TimeSpan.FromSeconds(settings.SequenceTimeoutSeconds));

        _hook.KeyPressed += OnKeyPressed;
        _hook.Install();

        RemoteControl.InitializeKioskSide();

        if (settings.ApplyPolicies)
        {
            _policies = new LockdownPolicies();
            _policies.Apply();
        }

        // Keep the display on and prevent sleep and the screen saver.
        SetThreadExecutionState(ES_CONTINUOUS | ES_DISPLAY_REQUIRED | ES_SYSTEM_REQUIRED);

        CreateForms();
        Cursor.Hide();
        SystemEvents.DisplaySettingsChanged += OnDisplaySettingsChanged;
        SystemEvents.SessionSwitch += OnSessionSwitch;

        _slideTimer = new Timer { Interval = (int)(settings.IntervalSeconds * 1000) };
        _slideTimer.Tick += (_, _) => ShowNext();
        _slideTimer.Start();

        _guardTimer = new Timer { Interval = 250 };
        _guardTimer.Tick += (_, _) =>
        {
            if (RemoteControl.IsStopRequested(_startedUtc))
                Shutdown();
            else
                KeepInFront();
        };
        _guardTimer.Start();

        ShowNext();
    }

    private void OnKeyPressed(Keys key)
    {
        // Runs inside the hook callback: defer the actual work so the callback returns quickly.
        if (_exitDetector.Feed(key))
            _sync.Post(_ => Shutdown(), null);
    }

    private void CreateForms()
    {
        foreach (var screen in Screen.AllScreens)
        {
            var form = new SlideForm(screen, _settings.Background);
            form.Show();
            _forms.Add(form);
        }
        MainForm = _forms.FirstOrDefault(f => f.Bounds == Screen.PrimaryScreen?.Bounds) ?? _forms[0];
        MainForm.Activate();
    }

    private void OnDisplaySettingsChanged(object? sender, EventArgs e) => RebuildForms();

    private void OnSessionSwitch(object? sender, SessionSwitchEventArgs e)
    {
        // Moving the session between the console and a Remote Desktop client changes the
        // monitor layout, and not every client resolution change raises DisplaySettingsChanged.
        if (e.Reason is SessionSwitchReason.RemoteConnect or SessionSwitchReason.ConsoleConnect or SessionSwitchReason.SessionUnlock)
            RebuildForms();
    }

    private void RebuildForms() => _sync.Post(_ =>
    {
        if (_shuttingDown)
            return;

        // Screens were added, removed or resized: rebuild one window per screen.
        var old = _forms.ToList();
        _forms.Clear();
        CreateForms();
        foreach (var form in old)
        {
            form.AllowClose = true;
            form.Close();
        }
        ApplyCurrent();
    }, null);

    private void KeepInFront()
    {
        // No foreground window while a Remote Desktop session is disconnected or locked.
        var foreground = GetForegroundWindow();
        if (foreground == IntPtr.Zero || MainForm is null)
            return;

        GetWindowThreadProcessId(foreground, out uint pid);
        if (pid == _processId)
            return;

        foreach (var form in _forms)
        {
            form.TopMost = true;
            form.BringToFront();
        }
        MainForm.Activate();
        SetForegroundWindow(MainForm.Handle);
    }

    private void ShowNext()
    {
        if (_index >= _files.Count)
        {
            _files = ScanImages();
            _index = 0;
        }

        // Skip files that cannot be decoded; give up after one full pass.
        for (int attempts = 0; attempts < _files.Count; attempts++)
        {
            string path = _files[_index];
            _index++;
            try
            {
                var previous = _current;
                _current = LoadImage(path);
                ApplyCurrent();
                previous?.Dispose();
                return;
            }
            catch (Exception ex) when (ex is IOException or ArgumentException or OutOfMemoryException or UnauthorizedAccessException)
            {
                Program.Log($"Kunde inte läsa bild {path}: {ex.Message}");
                if (_index >= _files.Count)
                    _index = 0;
            }
        }

        _current?.Dispose();
        _current = null;
        ApplyCurrent();
    }

    private void ApplyCurrent()
    {
        foreach (var form in _forms)
        {
            if (_current is not null)
                form.ShowImage(_current);
            else
                form.ShowMessage($"Inga bilder hittades i\n{_settings.ImageFolderPath}");
        }
    }

    private List<string> ScanImages()
    {
        string folder = _settings.ImageFolderPath;
        if (!Directory.Exists(folder))
            return [];

        var files = Directory.EnumerateFiles(folder)
            .Where(f => ImageExtensions.Contains(Path.GetExtension(f)))
            .Order(StringComparer.OrdinalIgnoreCase)
            .ToList();

        if (_settings.Shuffle)
            Random.Shared.Shuffle(System.Runtime.InteropServices.CollectionsMarshal.AsSpan(files));
        return files;
    }

    private static Image LoadImage(string path)
    {
        // Load via memory so the file is not locked and can be replaced while the kiosk runs.
        using var stream = new MemoryStream(File.ReadAllBytes(path));
        using var source = Image.FromStream(stream);
        var bitmap = new Bitmap(source);

        if (source.PropertyIdList.Contains(ExifOrientation))
        {
            var rotation = source.GetPropertyItem(ExifOrientation)?.Value?[0] switch
            {
                3 => RotateFlipType.Rotate180FlipNone,
                6 => RotateFlipType.Rotate90FlipNone,
                8 => RotateFlipType.Rotate270FlipNone,
                _ => RotateFlipType.RotateNoneFlipNone,
            };
            bitmap.RotateFlip(rotation);
        }
        return bitmap;
    }

    private void Shutdown()
    {
        if (_shuttingDown)
            return;
        _shuttingDown = true;

        _slideTimer.Stop();
        _guardTimer.Stop();
        SystemEvents.DisplaySettingsChanged -= OnDisplaySettingsChanged;
        SystemEvents.SessionSwitch -= OnSessionSwitch;
        _hook.Dispose();
        _policies?.Dispose();
        SetThreadExecutionState(ES_CONTINUOUS);
        Cursor.Show();

        foreach (var form in _forms)
        {
            form.AllowClose = true;
            form.Close();
        }
        ExitThread();
    }

    protected override void Dispose(bool disposing)
    {
        if (disposing)
        {
            // Also covers Windows shutdown, where the forms close without the exit sequence.
            Shutdown();
            _slideTimer.Dispose();
            _guardTimer.Dispose();
            _current?.Dispose();
        }
        base.Dispose(disposing);
    }
}
