using System.Text.Json;

namespace BlueLock;

internal sealed class KioskSettings
{
    public const int ExitSequenceLength = 5;

    public string ImageFolder { get; set; } = "Images";
    public double IntervalSeconds { get; set; } = 10;
    public bool Shuffle { get; set; }
    public string BackgroundColor { get; set; } = "Black";
    public string[] ExitSequence { get; set; } = ["F12", "F9", "F11", "F10", "Escape"];
    public double SequenceTimeoutSeconds { get; set; } = 5;
    public bool ApplyPolicies { get; set; } = true;

    public string ImageFolderPath => Path.GetFullPath(ImageFolder, AppContext.BaseDirectory);
    public Color Background => Color.FromName(BackgroundColor) is { IsKnownColor: true } c ? c : ColorTranslator.FromHtml(BackgroundColor);
    public Keys[] ExitKeys { get; private set; } = [];

    public static KioskSettings Load(string path)
    {
        var settings = File.Exists(path)
            ? JsonSerializer.Deserialize<KioskSettings>(File.ReadAllText(path), new JsonSerializerOptions
            {
                PropertyNameCaseInsensitive = true,
                ReadCommentHandling = JsonCommentHandling.Skip,
                AllowTrailingCommas = true,
            }) ?? new KioskSettings()
            : new KioskSettings();

        settings.Validate();
        return settings;
    }

    private void Validate()
    {
        // Refuse to start with a broken exit sequence, otherwise the kiosk could never be closed.
        if (ExitSequence is not { Length: ExitSequenceLength })
            throw new InvalidOperationException($"ExitSequence måste innehålla exakt {ExitSequenceLength} tangenter.");

        ExitKeys = ExitSequence.Select(name =>
        {
            if (!Enum.TryParse<Keys>(name, ignoreCase: true, out var key) || key == Keys.None)
                throw new InvalidOperationException($"Okänd tangent i ExitSequence: \"{name}\".");
            return KeyboardHook.Normalize(key);
        }).ToArray();

        if (IntervalSeconds < 1)
            throw new InvalidOperationException("IntervalSeconds måste vara minst 1.");
        if (SequenceTimeoutSeconds <= 0)
            throw new InvalidOperationException("SequenceTimeoutSeconds måste vara större än 0.");

        _ = Background;
    }
}
