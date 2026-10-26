using System.ComponentModel;
using System.Runtime.InteropServices;
using static BlueLock.NativeMethods;

namespace BlueLock;

/// <summary>
/// Global low-level keyboard hook that swallows every key press system-wide
/// (Win, Alt+Tab, Alt+F4, Ctrl+Esc, Ctrl+Shift+Esc, ...) and reports physical presses.
/// Ctrl+Alt+Del cannot be intercepted by any user-mode application; see LockdownPolicies.
/// </summary>
internal sealed class KeyboardHook : IDisposable
{
    // Auto-repeat keeps refreshing the timestamp, so a held key is reported once.
    // A key-up lost to a desktop switch (e.g. Ctrl+Alt+Del) expires after this delay.
    private const long RepeatWindowMs = 1500;

    private readonly LowLevelKeyboardProc _proc;
    private readonly Dictionary<uint, long> _down = [];
    private IntPtr _hook;

    public event Action<Keys>? KeyPressed;

    public KeyboardHook()
    {
        // Keep a reference to the delegate so the GC does not collect it while the hook is active.
        _proc = HookCallback;
    }

    public void Install()
    {
        if (_hook != IntPtr.Zero)
            return;

        _hook = SetWindowsHookExW(WH_KEYBOARD_LL, _proc, GetModuleHandleW(null), 0);
        if (_hook == IntPtr.Zero)
            throw new Win32Exception(Marshal.GetLastWin32Error(), "Kunde inte installera tangentbordshook.");
    }

    public void Dispose()
    {
        if (_hook != IntPtr.Zero)
        {
            UnhookWindowsHookEx(_hook);
            _hook = IntPtr.Zero;
        }
    }

    public static Keys Normalize(Keys key) => key switch
    {
        Keys.LShiftKey or Keys.RShiftKey or Keys.Shift => Keys.ShiftKey,
        Keys.LControlKey or Keys.RControlKey or Keys.Control => Keys.ControlKey,
        Keys.LMenu or Keys.RMenu or Keys.Alt => Keys.Menu,
        Keys.RWin => Keys.LWin,
        _ => key,
    };

    private IntPtr HookCallback(int nCode, IntPtr wParam, IntPtr lParam)
    {
        if (nCode < 0)
            return CallNextHookEx(_hook, nCode, wParam, lParam);

        var info = Marshal.PtrToStructure<KBDLLHOOKSTRUCT>(lParam);
        long now = Environment.TickCount64;

        switch ((int)wParam)
        {
            case WM_KEYDOWN or WM_SYSKEYDOWN:
                bool isRepeat = _down.TryGetValue(info.vkCode, out long last) && now - last < RepeatWindowMs;
                _down[info.vkCode] = now;
                if (!isRepeat)
                    KeyPressed?.Invoke(Normalize((Keys)info.vkCode));
                break;

            case WM_KEYUP or WM_SYSKEYUP:
                _down.Remove(info.vkCode);
                break;
        }

        // Swallow everything: no key reaches Windows or any other application.
        return 1;
    }
}
