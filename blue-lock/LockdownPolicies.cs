using System.Text.Json;
using Microsoft.Win32;

namespace BlueLock;

/// <summary>
/// Temporarily applies Windows policies that strip the Ctrl+Alt+Del screen
/// (Task Manager, lock, sign out, change password, switch user, shut down).
/// The original values are backed up to disk so they can be restored even after a crash.
/// </summary>
internal sealed class LockdownPolicies : IDisposable
{
    private const string SystemKey = @"Software\Microsoft\Windows\CurrentVersion\Policies\System";
    private const string ExplorerKey = @"Software\Microsoft\Windows\CurrentVersion\Policies\Explorer";

    private static readonly PolicyValue[] Policies =
    [
        new("HKCU", SystemKey, "DisableTaskMgr"),
        new("HKCU", SystemKey, "DisableLockWorkstation"),
        new("HKCU", SystemKey, "DisableChangePassword"),
        new("HKCU", ExplorerKey, "NoLogoff"),
        new("HKCU", ExplorerKey, "NoClose"),
        // Requires administrator rights; silently skipped otherwise.
        new("HKLM", SystemKey, "HideFastUserSwitching"),
    ];

    private static readonly string BackupPath = Path.Combine(Program.DataDirectory, "policy-backup.json");

    private Dictionary<string, int?>? _original;

    public void Apply()
    {
        // An existing backup means the previous run never restored; keep those true original values.
        _original = ReadBackup() ?? Policies.ToDictionary(p => p.Id, p => p.Read());
        Directory.CreateDirectory(Program.DataDirectory);
        File.WriteAllText(BackupPath, JsonSerializer.Serialize(_original));

        foreach (var policy in Policies)
            policy.TryWrite(1);
    }

    public void Dispose()
    {
        if (_original is null)
            return;

        foreach (var policy in Policies)
            policy.TryWrite(_original.GetValueOrDefault(policy.Id));

        File.Delete(BackupPath);
        _original = null;
    }

    /// <summary>Restores policies left behind by a run that crashed or was killed.</summary>
    public static void RestoreLeftovers()
    {
        if (ReadBackup() is { } backup)
            new LockdownPolicies { _original = backup }.Dispose();
    }

    private static Dictionary<string, int?>? ReadBackup()
    {
        try
        {
            return File.Exists(BackupPath)
                ? JsonSerializer.Deserialize<Dictionary<string, int?>>(File.ReadAllText(BackupPath))
                : null;
        }
        catch (JsonException)
        {
            return null;
        }
    }

    private sealed record PolicyValue(string Hive, string KeyPath, string Name)
    {
        public string Id => $@"{Hive}\{KeyPath}\{Name}";

        private RegistryKey Root => Hive == "HKLM" ? Registry.LocalMachine : Registry.CurrentUser;

        public int? Read()
        {
            using var key = Root.OpenSubKey(KeyPath);
            return key?.GetValue(Name) as int?;
        }

        public void TryWrite(int? value)
        {
            try
            {
                using var key = Root.CreateSubKey(KeyPath, writable: true);
                if (value is int v)
                    key.SetValue(Name, v, RegistryValueKind.DWord);
                else
                    key.DeleteValue(Name, throwOnMissingValue: false);
            }
            catch (Exception ex) when (ex is UnauthorizedAccessException or System.Security.SecurityException)
            {
            }
        }
    }
}
