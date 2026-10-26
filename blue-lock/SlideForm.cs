namespace BlueLock;

/// <summary>Borderless, topmost window covering one screen.</summary>
internal sealed class SlideForm : Form
{
    private readonly PictureBox _picture;
    private readonly Label _message;

    [System.ComponentModel.DesignerSerializationVisibility(System.ComponentModel.DesignerSerializationVisibility.Hidden)]
    public bool AllowClose { get; set; }

    public SlideForm(Screen screen, Color background)
    {
        FormBorderStyle = FormBorderStyle.None;
        StartPosition = FormStartPosition.Manual;
        Bounds = screen.Bounds;
        TopMost = true;
        ShowInTaskbar = false;
        ControlBox = false;
        BackColor = background;
        DoubleBuffered = true;

        _picture = new PictureBox
        {
            Dock = DockStyle.Fill,
            SizeMode = PictureBoxSizeMode.Zoom,
            BackColor = background,
        };
        _message = new Label
        {
            Dock = DockStyle.Fill,
            TextAlign = ContentAlignment.MiddleCenter,
            ForeColor = Color.Gray,
            BackColor = background,
            Font = new Font("Segoe UI", 20f),
            Visible = false,
        };

        Controls.Add(_picture);
        Controls.Add(_message);
    }

    public void ShowImage(Image image)
    {
        _message.Visible = false;
        _picture.Image = image;
    }

    public void ShowMessage(string text)
    {
        _picture.Image = null;
        _message.Text = text;
        _message.Visible = true;
    }

    protected override void OnFormClosing(FormClosingEventArgs e)
    {
        // Block Alt+F4, WM_CLOSE from other processes etc. Windows shutdown is allowed.
        if (!AllowClose && e.CloseReason != CloseReason.WindowsShutDown)
            e.Cancel = true;
        base.OnFormClosing(e);
    }
}
