using System.Diagnostics;
using System.Security;
using System.Security.AccessControl;
using System.Security.Principal;
using System.Text;
using System.Xml.Linq;
using static BlueLock.NativeMethods;

namespace BlueLock;

/// <summary>
/// Start, stop and query the kiosk from a non-interactive session such as an SSH login.
/// Processes started from SSH cannot show windows on any desktop, so the kiosk is launched
/// through a scheduled task that runs in the task user's interactive session — at the
/// physical console or over Remote Desktop.
/// Stopping is signalled with a file the kiosk polls for, so it can restore its policies cleanly.
/// </summary>
internal static class RemoteControl
{
    private const string TaskName = "BlueLock";
    private static readonly TimeSpan StartTimeout = TimeSpan.FromSeconds(10);
    private static readonly TimeSpan StopTimeout = TimeSpan.FromSeconds(10);

    private static string ControlDirectory { get; } =
        Path.Combine(Environment.GetFolderPath(Environment.SpecialFolder.CommonApplicationData), "BlueLock");

    private static string StopSignalPath => Path.Combine(ControlDirectory, "stop.signal");

    private sealed record UserSession(uint Id, string User, bool IsActive, bool IsRemote)
    {
        public string Description =>
            $"session {Id}, {(IsRemote ? "fjärrskrivbord" : "skärmen")}{(IsActive ? "" : ", frånkopplad")}";
    }

    public static bool IsSshSession =>
        !string.IsNullOrEmpty(Environment.GetEnvironmentVariable("SSH_CONNECTION")) ||
        !string.IsNullOrEmpty(Environment.GetEnvironmentVariable("SSH_CLIENT"));

    /// <param name="autostart">Also start the kiosk at logon and whenever someone connects, locally or over Remote Desktop.</param>
    public static int RegisterTask(string? user, bool autostart)
    {
        user ??= WindowsIdentity.GetCurrent().Name;
        string exe = Environment.ProcessPath ?? throw new InvalidOperationException("Hittar inte sökvägen till exe-filen.");
        string userXml = SecurityElement.Escape(user);

        string triggers = autostart
            ? $"""
              <Triggers>
                <LogonTrigger>
                  <UserId>{userXml}</UserId>
                </LogonTrigger>
                <SessionStateChangeTrigger>
                  <StateChange>RemoteConnect</StateChange>
                  <UserId>{userXml}</UserId>
                </SessionStateChangeTrigger>
                <SessionStateChangeTrigger>
                  <StateChange>ConsoleConnect</StateChange>
                  <UserId>{userXml}</UserId>
                </SessionStateChangeTrigger>
              </Triggers>
              """
            : "";

        string xml = $"""
            <?xml version="1.0" encoding="UTF-16"?>
            <Task version="1.2" xmlns="http://schemas.microsoft.com/windows/2004/02/mit/task">
              <RegistrationInfo>
                <Description>Startar BlueLock-kiosken i användarens session, vid skärmen eller via fjärrskrivbord.</Description>
              </RegistrationInfo>
              {triggers}
              <Principals>
                <Principal id="Author">
                  <UserId>{userXml}</UserId>
                  <LogonType>InteractiveToken</LogonType>
                  <RunLevel>LeastPrivilege</RunLevel>
                </Principal>
              </Principals>
              <Settings>
                <MultipleInstancesPolicy>IgnoreNew</MultipleInstancesPolicy>
                <DisallowStartIfOnBatteries>false</DisallowStartIfOnBatteries>
                <StopIfGoingOnBatteries>false</StopIfGoingOnBatteries>
                <AllowStartOnDemand>true</AllowStartOnDemand>
                <ExecutionTimeLimit>PT0S</ExecutionTimeLimit>
                <Priority>4</Priority>
                <Enabled>true</Enabled>
              </Settings>
              <Actions Context="Author">
                <Exec>
                  <Command>{SecurityElement.Escape(exe)}</Command>
                  <WorkingDirectory>{SecurityElement.Escape(AppContext.BaseDirectory)}</WorkingDirectory>
                </Exec>
              </Actions>
            </Task>
            """;

        string xmlPath = Path.Combine(Path.GetTempPath(), $"BlueLock-task-{Guid.NewGuid():N}.xml");
        try
        {
            File.WriteAllText(xmlPath, xml, Encoding.Unicode);
            var (exitCode, _) = Schtasks("/Create", "/TN", TaskName, "/XML", xmlPath, "/F");
            if (exitCode == 0)
            {
                Console.WriteLine($"Aktiviteten \"{TaskName}\" är registrerad för {user}.");
                if (autostart)
                    Console.WriteLine("Kiosken startar automatiskt vid inloggning och vid anslutning via skärmen eller fjärrskrivbord.");
            }
            else if (autostart)
            {
                Console.Error.WriteLine("Inloggnings- och anslutningsutlösare kan kräva administratörsbehörighet.");
            }
            return exitCode;
        }
        finally
        {
            File.Delete(xmlPath);
        }
    }

    public static int UnregisterTask() => Schtasks("/Delete", "/TN", TaskName, "/F").ExitCode;

    public static int Start()
    {
        if (FindKioskProcesses() is { Count: > 0 } running)
        {
            Console.WriteLine($"Kiosken körs redan ({DescribeProcesses(running)}).");
            return 0;
        }

        if (GetTaskUser() is not { } taskUser)
        {
            Console.Error.WriteLine("Aktiviteten saknas. Registrera den först med: BlueLock.exe --register-task");
            return 5;
        }

        var sessions = GetUserSessions().Where(s => SameAccount(s.User, taskUser)).ToList();
        if (sessions.Count == 0)
        {
            Console.Error.WriteLine($"{taskUser} är inte inloggad, varken vid skärmen eller via fjärrskrivbord. Kiosken kan bara starta i en inloggad session.");
            return 2;
        }
        if (!sessions.Any(s => s.IsActive))
            Console.WriteLine($"Obs: {taskUser} är frånkopplad. Kiosken syns när någon ansluter igen.");

        var (exitCode, _) = Schtasks("/Run", "/TN", TaskName);
        if (exitCode != 0)
            return exitCode;

        // Wait briefly so we can report whether, and where, the kiosk actually came up.
        var deadline = DateTime.UtcNow + StartTimeout;
        while (DateTime.UtcNow < deadline)
        {
            if (FindKioskProcesses() is { Count: > 0 } started)
            {
                Console.WriteLine($"Kiosken startade för {taskUser} ({DescribeProcesses(started)}).");
                return 0;
            }
            Thread.Sleep(250);
        }

        Console.Error.WriteLine($"Aktiviteten startades men kiosken syns inte i {taskUser}s session.");
        return 3;
    }

    public static int Stop()
    {
        if (FindKioskProcesses().Count == 0)
        {
            Console.WriteLine("Kiosken körs inte.");
            return 0;
        }

        EnsureControlDirectory();
        File.WriteAllText(StopSignalPath, DateTime.UtcNow.ToString("O"));

        var deadline = DateTime.UtcNow + StopTimeout;
        while (DateTime.UtcNow < deadline)
        {
            if (FindKioskProcesses().Count == 0)
            {
                TryDelete(StopSignalPath);
                Console.WriteLine("Kiosken är stoppad.");
                return 0;
            }
            Thread.Sleep(250);
        }

        Console.Error.WriteLine("Kiosken svarade inte på stoppsignalen.");
        return 4;
    }

    public static int Status()
    {
        var running = FindKioskProcesses();
        Console.WriteLine(running.Count == 0
            ? "Kiosken körs inte."
            : $"Kiosken körs ({DescribeProcesses(running)}).");

        var sessions = GetUserSessions();
        Console.WriteLine(sessions.Count == 0 ? "Inloggade: ingen." : "Inloggade:");
        foreach (var session in sessions)
        {
            bool hasKiosk = running.Any(p => p.SessionId == session.Id);
            Console.WriteLine($"  {session.User} ({session.Description}){(hasKiosk ? " – kiosk" : "")}");
        }

        Console.WriteLine($"Aktivitetens användare: {GetTaskUser() ?? "aktiviteten saknas"}.");
        return running.Count == 0 ? 1 : 0;
    }

    /// <summary>Called by the running kiosk: prepares the control directory and clears stale signals.</summary>
    public static void InitializeKioskSide()
    {
        EnsureControlDirectory();
        TryDelete(StopSignalPath);
    }

    /// <summary>Called by the running kiosk: true when a stop was requested after the kiosk started.</summary>
    public static bool IsStopRequested(DateTime startedUtc)
    {
        try
        {
            // The signal may belong to another account and be undeletable, so compare timestamps too.
            if (!File.Exists(StopSignalPath) || File.GetLastWriteTimeUtc(StopSignalPath) < startedUtc)
                return false;
        }
        catch (IOException)
        {
            return false;
        }

        TryDelete(StopSignalPath);
        return true;
    }

    private static void EnsureControlDirectory()
    {
        var directory = Directory.CreateDirectory(ControlDirectory);
        try
        {
            // The kiosk account and the SSH account may differ; both must be able to write signals.
            var security = directory.GetAccessControl();
            security.AddAccessRule(new FileSystemAccessRule(
                new SecurityIdentifier(WellKnownSidType.AuthenticatedUserSid, null),
                FileSystemRights.Modify,
                InheritanceFlags.ContainerInherit | InheritanceFlags.ObjectInherit,
                PropagationFlags.None,
                AccessControlType.Allow));
            directory.SetAccessControl(security);
        }
        catch (UnauthorizedAccessException)
        {
        }
    }

    private static List<Process> FindKioskProcesses()
    {
        int self = Environment.ProcessId;
        return Process.GetProcessesByName(Path.GetFileNameWithoutExtension(Environment.ProcessPath) ?? "BlueLock")
            .Where(p => p.Id != self)
            .ToList();
    }

    private static string DescribeProcesses(List<Process> processes)
    {
        var sessions = GetUserSessions();
        return string.Join("; ", processes.Select(p =>
            sessions.FirstOrDefault(s => s.Id == p.SessionId) is { } s
                ? $"PID {p.Id}, {s.Description}"
                : $"PID {p.Id}, session {p.SessionId}"));
    }

    /// <summary>Interactive sessions with a logged-on user: the console and Remote Desktop sessions.</summary>
    private static List<UserSession> GetUserSessions()
    {
        var sessions = new List<UserSession>();
        foreach (var (id, state) in EnumerateSessions())
        {
            if (state is not (WTS_CONNECTSTATE_CLASS.WTSActive or WTS_CONNECTSTATE_CLASS.WTSDisconnected))
                continue;

            string? user = QuerySessionString(id, WTS_INFO_CLASS.WTSUserName);
            if (string.IsNullOrEmpty(user))
                continue;

            string? domain = QuerySessionString(id, WTS_INFO_CLASS.WTSDomainName);
            sessions.Add(new UserSession(
                id,
                string.IsNullOrEmpty(domain) ? user : $@"{domain}\{user}",
                state == WTS_CONNECTSTATE_CLASS.WTSActive,
                IsRemoteDesktopSession(id)));
        }
        return sessions;
    }

    /// <summary>The account the scheduled task runs as, or null when the task is not registered.</summary>
    private static string? GetTaskUser()
    {
        var (exitCode, output) = Schtasks(reportErrors: false, "/Query", "/TN", TaskName, "/XML", "ONE");
        if (exitCode != 0)
            return null;

        try
        {
            var userId = XDocument.Parse(output).Descendants().FirstOrDefault(e => e.Name.LocalName == "UserId")?.Value;
            // Task Scheduler may store the principal as a SID; show it as DOMAIN\user.
            return userId is not null && userId.StartsWith("S-1-", StringComparison.Ordinal) && TryGetSid(userId) is { } sid
                ? TryGetAccountName(sid) ?? userId
                : userId;
        }
        catch (System.Xml.XmlException)
        {
            return null;
        }
    }

    private static bool SameAccount(string a, string b) =>
        string.Equals(a, b, StringComparison.OrdinalIgnoreCase) ||
        (TryGetSid(a) is { } sidA && sidA == TryGetSid(b));

    private static SecurityIdentifier? TryGetSid(string account)
    {
        try
        {
            return account.StartsWith("S-1-", StringComparison.Ordinal)
                ? new SecurityIdentifier(account)
                : (SecurityIdentifier)new NTAccount(account).Translate(typeof(SecurityIdentifier));
        }
        catch (SystemException)
        {
            // Unknown account name or malformed SID.
            return null;
        }
    }

    private static string? TryGetAccountName(SecurityIdentifier sid)
    {
        try
        {
            return sid.Translate(typeof(NTAccount)).Value;
        }
        catch (IdentityNotMappedException)
        {
            return null;
        }
    }

    private static (int ExitCode, string Output) Schtasks(params string[] args) => Schtasks(reportErrors: true, args);

    private static (int ExitCode, string Output) Schtasks(bool reportErrors, params string[] args)
    {
        var startInfo = new ProcessStartInfo("schtasks.exe")
        {
            UseShellExecute = false,
            RedirectStandardOutput = true,
            RedirectStandardError = true,
            CreateNoWindow = true,
        };
        foreach (var arg in args)
            startInfo.ArgumentList.Add(arg);

        using var process = Process.Start(startInfo)!;
        var errorTask = process.StandardError.ReadToEndAsync();
        string output = process.StandardOutput.ReadToEnd();
        string error = errorTask.Result;
        process.WaitForExit();

        if (process.ExitCode != 0 && reportErrors)
            Console.Error.Write(error.Length > 0 ? error : output);
        return (process.ExitCode, output);
    }

    private static void TryDelete(string path)
    {
        try
        {
            File.Delete(path);
        }
        catch (Exception ex) when (ex is IOException or UnauthorizedAccessException)
        {
        }
    }
}
