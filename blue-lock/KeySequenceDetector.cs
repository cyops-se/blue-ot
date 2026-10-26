namespace BlueLock;

/// <summary>
/// Detects a fixed sequence of key presses. The buffer resets when the pause
/// between two presses exceeds the timeout.
/// </summary>
internal sealed class KeySequenceDetector(Keys[] sequence, TimeSpan timeout)
{
    private readonly List<Keys> _buffer = new(sequence.Length);
    private long _lastPress;

    public bool Feed(Keys key)
    {
        long now = Environment.TickCount64;
        if (now - _lastPress > timeout.TotalMilliseconds)
            _buffer.Clear();
        _lastPress = now;

        _buffer.Add(key);
        if (_buffer.Count > sequence.Length)
            _buffer.RemoveAt(0);

        if (_buffer.Count == sequence.Length && _buffer.SequenceEqual(sequence))
        {
            _buffer.Clear();
            return true;
        }
        return false;
    }
}
