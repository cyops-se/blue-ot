using System.Diagnostics;
using System.Text;

namespace BlueLock;

/// <summary>
/// Append-only log of the kiosk's life cycle: installation, activation, deactivation and removal.
/// It lives in the shared control directory so every account (kiosk, SSH, admin) writes to the
/// same file, which can be followed with <c>Get-Content -Wait</c>.
/// </summary>
internal static class StateLog
{
    public const string Installed = "INSTALLERAD";
    public const string Uninstalled = "AVINSTALLERAD";
    public const string AutostartOn = "AUTOSTART-PÅ";
    public const string AutostartOff = "AUTOSTART-AV";
    public const string StartRequested = "STARTBEGÄRAN";
    public const string StartFailed = "START-MISSLYCKAD";
    public const string Activated = "AKTIVERAD";
    public const string StopRequested = "STOPPBEGÄRAN";
    public const string Deactivated = "DEAKTIVERAD";
    public const string Crashed = "KRASCH";
    public const string PoliciesRestored = "POLICYS-ÅTERSTÄLLDA";
    public const string Error = "FEL";

    public static string FilePath => Path.Combine(RemoteControl.ControlDirectory, "state.log");

    private static bool _directoryReady;

    public static void Write(string state, string? details = null)
    {
        string line = $"{DateTimeOffset.Now:yyyy-MM-dd HH:mm:ss zzz}  {state,-19}  " +
                      $@"{Environment.UserDomainName}\{Environment.UserName}, session {Process.GetCurrentProcess().SessionId}, PID {Environment.ProcessId}" +
                      (string.IsNullOrEmpty(details) ? "" : $"  {details}") + Environment.NewLine;
        byte[] bytes = Encoding.UTF8.GetBytes(line);

        // Kiosk and SSH commands may write at the same time: open exclusively for writing and retry on conflicts.
        for (int attempt = 0; attempt < 20; attempt++)
        {
            try
            {
                if (!_directoryReady)
                {
                    RemoteControl.EnsureControlDirectory();
                    _directoryReady = true;
                }
                using var stream = new FileStream(FilePath, FileMode.Append, FileAccess.Write, FileShare.Read | FileShare.Delete);
                stream.Write(bytes);
                return;
            }
            catch (IOException)
            {
                Thread.Sleep(25);
            }
            catch (UnauthorizedAccessException)
            {
                break;
            }
        }
        Program.Log($"Kunde inte skriva tillstånd till {FilePath}: {line.TrimEnd()}");
    }
}
