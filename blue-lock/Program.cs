using Microsoft.Win32;

namespace BlueLock;

internal static class Program
{
    private const string RunKey = @"Software\Microsoft\Windows\CurrentVersion\Run";
    private const string AppName = "BlueLock";

    public static string DataDirectory { get; } =
        Path.Combine(Environment.GetFolderPath(Environment.SpecialFolder.LocalApplicationData), AppName);

    [STAThread]
    private static int Main(string[] args)
    {
        string? command = args.FirstOrDefault()?.ToLowerInvariant();

        // Over SSH a plain start would end up in the invisible SSH session; launch on the console instead.
        if (command is null && RemoteControl.IsSshSession)
            command = "--start";

        if (command is not null)
            AttachParentConsole();

        switch (command)
        {
            case "--register-task":
                return RemoteControl.RegisterTask(
                    user: args.Skip(1).FirstOrDefault(a => !a.StartsWith("--", StringComparison.Ordinal)),
                    autostart: args.Contains("--autostart", StringComparer.OrdinalIgnoreCase));

            case "--unregister-task":
                return RemoteControl.UnregisterTask();

            case "--start":
                return RemoteControl.Start();

            case "--stop":
                return RemoteControl.Stop();

            case "--status":
                return RemoteControl.Status();

            case "--install-autostart":
                using (var key = Registry.CurrentUser.CreateSubKey(RunKey))
                    key.SetValue(AppName, $"\"{Environment.ProcessPath}\"");
                return 0;

            case "--remove-autostart":
                using (var key = Registry.CurrentUser.CreateSubKey(RunKey))
                    key.DeleteValue(AppName, throwOnMissingValue: false);
                return 0;

            case "--restore-policies":
                LockdownPolicies.RestoreLeftovers();
                return 0;

            case not null:
                Console.Error.WriteLine($"Okänt kommando: {command}");
                return 1;
        }

        using var mutex = new Mutex(true, @"Local\BlueLock.Kiosk", out bool isFirstInstance);
        if (!isFirstInstance)
            return 0;

        ApplicationConfiguration.Initialize();

        KioskSettings settings;
        try
        {
            settings = KioskSettings.Load(Path.Combine(AppContext.BaseDirectory, "appsettings.json"));
        }
        catch (Exception ex)
        {
            MessageBox.Show($"Felaktig konfiguration i appsettings.json:\n\n{ex.Message}", AppName,
                MessageBoxButtons.OK, MessageBoxIcon.Error);
            return 1;
        }

        // A kiosk must keep running: log UI-thread errors instead of crashing.
        Application.SetUnhandledExceptionMode(UnhandledExceptionMode.CatchException);
        Application.ThreadException += (_, e) => Log(e.Exception.ToString());
        AppDomain.CurrentDomain.UnhandledException += (_, e) =>
        {
            Log(e.ExceptionObject.ToString() ?? "Okänt fel");
            LockdownPolicies.RestoreLeftovers();
        };

        using var context = new KioskContext(settings);
        Application.Run(context);
        return 0;
    }

    /// <summary>
    /// This is a GUI-subsystem exe, so command-line output only shows up when stdout was
    /// inherited (e.g. a pipe from sshd) or after attaching to the parent's console.
    /// </summary>
    private static void AttachParentConsole()
    {
        // Keep inherited (redirected) handles; attach only to learn the parent's code page
        // and to get output at all when started from an interactive console.
        IntPtr stdout = NativeMethods.GetStdHandle(NativeMethods.STD_OUTPUT_HANDLE);
        IntPtr stderr = NativeMethods.GetStdHandle(NativeMethods.STD_ERROR_HANDLE);
        bool attached = NativeMethods.AttachConsole(NativeMethods.ATTACH_PARENT_PROCESS);
        if (stdout != IntPtr.Zero)
            NativeMethods.SetStdHandle(NativeMethods.STD_OUTPUT_HANDLE, stdout);
        if (stderr != IntPtr.Zero)
            NativeMethods.SetStdHandle(NativeMethods.STD_ERROR_HANDLE, stderr);

        // SSH clients expect UTF-8; otherwise follow the parent console, falling back to OEM.
        System.Text.Encoding.RegisterProvider(System.Text.CodePagesEncodingProvider.Instance);
        int codePage = RemoteControl.IsSshSession ? 65001
            : attached && NativeMethods.GetConsoleOutputCP() is > 0 and var cp ? (int)cp
            : System.Globalization.CultureInfo.CurrentCulture.TextInfo.OEMCodePage;
        var encoding = codePage == 65001
            ? new System.Text.UTF8Encoding(encoderShouldEmitUTF8Identifier: false)
            : System.Text.Encoding.GetEncoding(codePage);
        Console.SetOut(new StreamWriter(Console.OpenStandardOutput(), encoding) { AutoFlush = true });
        Console.SetError(new StreamWriter(Console.OpenStandardError(), encoding) { AutoFlush = true });
    }

    public static void Log(string message)
    {
        try
        {
            Directory.CreateDirectory(DataDirectory);
            File.AppendAllText(Path.Combine(DataDirectory, "kiosk.log"), $"{DateTime.Now:yyyy-MM-dd HH:mm:ss} {message}{Environment.NewLine}");
        }
        catch (IOException)
        {
        }
    }
}
