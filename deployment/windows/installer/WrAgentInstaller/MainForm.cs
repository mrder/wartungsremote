using System.Diagnostics;
using System.Reflection;
using System.Security.Principal;

namespace WrAgentInstaller;

public class MainForm : Form
{
    private readonly TextBox _serverUrlBox;
    private readonly TextBox _tokenBox;
    private readonly CheckBox _showTokenBox;
    private readonly Button _installButton;
    private readonly TextBox _logBox;
    private readonly Label _statusLabel;

    private const string AdminGroupSid = "*S-1-5-32-544";
    private const string SystemSid = "*S-1-5-18";

    public MainForm()
    {
        Text = "WartungsRemote Agent Setup";
        ClientSize = new Size(520, 420);
        FormBorderStyle = FormBorderStyle.FixedDialog;
        MaximizeBox = false;
        StartPosition = FormStartPosition.CenterScreen;

        var title = new Label
        {
            Text = "WartungsRemote Agent installieren",
            Font = new Font(Font.FontFamily, 13, FontStyle.Bold),
            Left = 20,
            Top = 16,
            AutoSize = true,
        };

        var intro = new Label
        {
            Text = "Trägt diesen Computer bei eurer WartungsRemote-Instanz ein und installiert den Agent als Windows-Dienst.",
            Left = 20,
            Top = 46,
            Width = 480,
            Height = 36,
        };

        var serverLabel = new Label { Text = "Server-URL", Left = 20, Top = 92, AutoSize = true };
        _serverUrlBox = new TextBox { Left = 20, Top = 112, Width = 480, PlaceholderText = "https://service.example.de" };

        var tokenLabel = new Label { Text = "Enrollment-Token (optional)", Left = 20, Top = 144, AutoSize = true };
        _tokenBox = new TextBox { Left = 20, Top = 164, Width = 480, UseSystemPasswordChar = true };
        _showTokenBox = new CheckBox { Text = "Token anzeigen", Left = 20, Top = 192, AutoSize = true };
        _showTokenBox.CheckedChanged += (_, _) => _tokenBox.UseSystemPasswordChar = !_showTokenBox.Checked;

        _installButton = new Button { Text = "Installieren", Left = 20, Top = 226, Width = 140, Height = 32 };
        _installButton.Click += async (_, _) => await RunInstallAsync();

        _statusLabel = new Label { Left = 170, Top = 232, Width = 330, AutoSize = false, Height = 20 };

        _logBox = new TextBox
        {
            Left = 20,
            Top = 266,
            Width = 480,
            Height = 130,
            Multiline = true,
            ReadOnly = true,
            ScrollBars = ScrollBars.Vertical,
            Font = new Font(FontFamily.GenericMonospace, 8.5f),
        };

        Controls.AddRange(new Control[]
        {
            title, intro, serverLabel, _serverUrlBox, tokenLabel, _tokenBox, _showTokenBox,
            _installButton, _statusLabel, _logBox,
        });

        PrefillFromCommandLine();
    }

    private void PrefillFromCommandLine()
    {
        var args = Environment.GetCommandLineArgs();
        for (var i = 1; i < args.Length - 1; i++)
        {
            switch (args[i])
            {
                case "--server-url":
                    _serverUrlBox.Text = args[i + 1];
                    break;
                case "--token":
                    _tokenBox.Text = args[i + 1];
                    break;
            }
        }
    }

    private void Log(string message)
    {
        _logBox.AppendText(message + Environment.NewLine);
    }

    private async Task RunInstallAsync()
    {
        var serverUrl = _serverUrlBox.Text.Trim();
        var token = _tokenBox.Text.Trim();

        if (!serverUrl.StartsWith("http://", StringComparison.OrdinalIgnoreCase) &&
            !serverUrl.StartsWith("https://", StringComparison.OrdinalIgnoreCase))
        {
            MessageBox.Show(this, "Bitte eine gültige Server-URL angeben (https://...).", "WartungsRemote Agent Setup",
                MessageBoxButtons.OK, MessageBoxIcon.Warning);
            return;
        }

        if (!IsRunningAsAdministrator())
        {
            MessageBox.Show(this,
                "Dieses Setup muss mit Administratorrechten laufen. Bitte die Datei erneut per Rechtsklick \"Als Administrator ausführen\" starten.",
                "WartungsRemote Agent Setup", MessageBoxButtons.OK, MessageBoxIcon.Error);
            return;
        }

        _installButton.Enabled = false;
        _logBox.Clear();
        _statusLabel.Text = "Installation läuft...";

        try
        {
            await Task.Run(() => Install(serverUrl, token));
            _statusLabel.Text = "Fertig.";
            Log("");
            Log("Installation abgeschlossen. Status prüfen mit: Get-Service wartungsremote-agent");
            MessageBox.Show(this, "wr-agent wurde installiert und gestartet.", "WartungsRemote Agent Setup",
                MessageBoxButtons.OK, MessageBoxIcon.Information);
        }
        catch (Exception ex)
        {
            _statusLabel.Text = "Fehlgeschlagen.";
            Log("");
            Log("FEHLER: " + ex.Message);
            MessageBox.Show(this, "Installation fehlgeschlagen:\n" + ex.Message, "WartungsRemote Agent Setup",
                MessageBoxButtons.OK, MessageBoxIcon.Error);
        }
        finally
        {
            _installButton.Enabled = true;
        }
    }

    private static bool IsRunningAsAdministrator()
    {
        using var identity = WindowsIdentity.GetCurrent();
        var principal = new WindowsPrincipal(identity);
        return principal.IsInRole(WindowsBuiltInRole.Administrator);
    }

    private void Install(string serverUrl, string token)
    {
        var installDir = Path.Combine(Environment.GetFolderPath(Environment.SpecialFolder.ProgramFiles), "WartungsRemote");
        var baseData = Path.Combine(Environment.GetFolderPath(Environment.SpecialFolder.CommonApplicationData), "WartungsRemote");
        var configDir = Path.Combine(baseData, "config");
        var dataDir = Path.Combine(baseData, "data");
        var logDir = Path.Combine(baseData, "logs");

        RunOnUi(() => Log($"Installationsverzeichnis: {installDir}"));
        Directory.CreateDirectory(installDir);
        Directory.CreateDirectory(configDir);
        Directory.CreateDirectory(dataDir);
        Directory.CreateDirectory(logDir);

        var exePath = Path.Combine(installDir, "wr-agent.exe");
        if (File.Exists(exePath))
        {
            RunOnUi(() => Log("Bestehende Installation gefunden, stoppe Dienst für Upgrade..."));
            TryRunProcess(exePath, "--service stop");
            TryRunProcess(exePath, "--service uninstall");
        }

        RunOnUi(() => Log("Extrahiere wr-agent.exe..."));
        ExtractEmbeddedAgent(exePath);

        var configPath = Path.Combine(configDir, "agent.yaml");
        if (!File.Exists(configPath))
        {
            RunOnUi(() => Log("Schreibe agent.yaml..."));
            var yaml = "server_url: " + serverUrl + "\n" +
                       "update_channel: stable\n" +
                       "log_level: info\n" +
                       "policy:\n" +
                       "  terminal: true\n" +
                       "  ssh_tunnel: true\n" +
                       "  rdp_tunnel: true\n" +
                       "  files_read: true\n" +
                       "  files_write: true\n" +
                       "  service_control: true\n" +
                       "  process_terminate: true\n" +
                       "  power_control: true\n";
            File.WriteAllText(configPath, yaml);
        }
        else
        {
            // Keep any hand-tuned policy flags, but never let server_url go
            // stale on a reinstall/upgrade — a customer moving to a new
            // server, or re-running this with a corrected URL, must actually
            // take effect.
            RunOnUi(() => Log("Aktualisiere server_url in bestehender agent.yaml..."));
            var lines = File.ReadAllLines(configPath);
            for (var i = 0; i < lines.Length; i++)
            {
                if (lines[i].StartsWith("server_url:", StringComparison.Ordinal))
                {
                    lines[i] = "server_url: " + serverUrl;
                }
            }
            File.WriteAllLines(configPath, lines);
        }

        if (!string.IsNullOrEmpty(token))
        {
            RunOnUi(() => Log("Schreibe Enrollment-Token..."));
            File.WriteAllText(Path.Combine(dataDir, "enroll.token"), token);

            // A freshly supplied token means "(re-)enroll this device" — an
            // old stored identity from a previous enrollment must not
            // silently win and make the new token look like it did nothing.
            var credentialFile = Path.Combine(dataDir, "device_credential.dat");
            if (File.Exists(credentialFile))
            {
                RunOnUi(() => Log("Neues Token angegeben, entferne bisherige Geräte-Identität für Neu-Enrollment..."));
                File.Delete(credentialFile);
            }
        }

        RunOnUi(() => Log("Setze Berechtigungen (icacls)..."));
        RunProcess("icacls.exe", $"\"{baseData}\" /inheritance:r");
        RunProcess("icacls.exe", $"\"{baseData}\" /grant:r \"{SystemSid}:(OI)(CI)F\" \"{AdminGroupSid}:(OI)(CI)F\"");

        RunOnUi(() => Log("Installiere Windows-Dienst..."));
        RunProcess(exePath, "--service install");
        RunOnUi(() => Log("Starte Dienst..."));
        RunProcess(exePath, "--service start");
    }

    private void ExtractEmbeddedAgent(string destinationPath)
    {
        var assembly = Assembly.GetExecutingAssembly();
        var resourceName = assembly.GetManifestResourceNames()
            .FirstOrDefault(n => n.EndsWith("wr-agent.exe", StringComparison.OrdinalIgnoreCase));
        if (resourceName == null)
        {
            throw new InvalidOperationException("wr-agent.exe wurde nicht in dieses Setup eingebettet.");
        }

        using var resourceStream = assembly.GetManifestResourceStream(resourceName)
            ?? throw new InvalidOperationException("Eingebettete wr-agent.exe konnte nicht gelesen werden.");
        using var fileStream = File.Create(destinationPath);
        resourceStream.CopyTo(fileStream);
    }

    private void TryRunProcess(string fileName, string arguments)
    {
        try
        {
            RunProcess(fileName, arguments);
        }
        catch (Exception ex)
        {
            RunOnUi(() => Log($"(ignoriert) {fileName} {arguments}: {ex.Message}"));
        }
    }

    private void RunProcess(string fileName, string arguments)
    {
        var psi = new ProcessStartInfo(fileName, arguments)
        {
            UseShellExecute = false,
            RedirectStandardOutput = true,
            RedirectStandardError = true,
            CreateNoWindow = true,
        };
        using var process = Process.Start(psi) ?? throw new InvalidOperationException($"Konnte {fileName} nicht starten.");
        var stdout = process.StandardOutput.ReadToEnd();
        var stderr = process.StandardError.ReadToEnd();
        process.WaitForExit();

        if (!string.IsNullOrWhiteSpace(stdout)) RunOnUi(() => Log(stdout.TrimEnd()));
        if (!string.IsNullOrWhiteSpace(stderr)) RunOnUi(() => Log(stderr.TrimEnd()));

        if (process.ExitCode != 0)
        {
            throw new InvalidOperationException($"{fileName} {arguments} beendete sich mit Code {process.ExitCode}");
        }
    }

    private void RunOnUi(Action action)
    {
        if (InvokeRequired)
        {
            base.Invoke(action);
        }
        else
        {
            action();
        }
    }
}
