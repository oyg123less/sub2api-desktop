package codexremote

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"path"
	"strings"

	"sub2api-desktop/core/internal/codexcfg"
)

type sshRemoteOperations struct{ connection remoteConnection }

func (r *sshRemoteOperations) Probe(ctx context.Context) (Probe, error) {
	output, err := r.connection.Run(ctx, `printf '%s\n' "$(uname -s)" "$HOME"`, nil)
	if err != nil {
		return Probe{}, codedError("remote_command_failed", err)
	}
	lines := strings.Split(strings.ReplaceAll(strings.TrimSpace(string(output)), "\r\n", "\n"), "\n")
	if len(lines) < 2 {
		return Probe{}, codedError("remote_command_failed", fmt.Errorf("unexpected probe output"))
	}
	osName, home := strings.TrimSpace(lines[0]), strings.TrimSpace(lines[1])
	if osName != "Linux" && osName != "Darwin" {
		return Probe{}, codedError("unsupported_os", nil)
	}
	if home == "" || !strings.HasPrefix(home, "/") {
		return Probe{}, codedError("remote_command_failed", fmt.Errorf("invalid remote home"))
	}
	return Probe{OS: osName, Home: home, CodexDir: path.Join(home, ".codex")}, nil
}

func (r *sshRemoteOperations) Inject(ctx context.Context, codexDir, config, auth string) error {
	if err := codexcfg.ValidateConfig(config); err != nil {
		return err
	}
	if err := codexcfg.ValidateAuth(auth); err != nil {
		return err
	}
	configPath, authPath := path.Join(codexDir, "config.toml"), path.Join(codexDir, "auth.json")
	prepare := fmt.Sprintf(`set -eu
mkdir -p %s
chmod 700 %s
for file in %s %s; do
  if [ ! -e "$file.sub2api-bak" ] && [ ! -e "$file.sub2api-absent" ]; then
    if [ -e "$file" ]; then cp -p "$file" "$file.sub2api-bak"; chmod 600 "$file.sub2api-bak"; else : > "$file.sub2api-absent"; chmod 600 "$file.sub2api-absent"; fi
  fi
done`, shellQuote(codexDir), shellQuote(codexDir), shellQuote(configPath), shellQuote(authPath))
	if _, err := r.connection.Run(ctx, prepare, nil); err != nil {
		return codedError("remote_command_failed", err)
	}
	suffix := randomSuffix()
	configTemp, authTemp := configPath+".sub2api-tmp-"+suffix, authPath+".sub2api-tmp-"+suffix
	if err := r.writeTemp(ctx, configTemp, []byte(config)); err != nil {
		return err
	}
	if err := r.writeTemp(ctx, authTemp, []byte(auth)); err != nil {
		_, _ = r.connection.Run(ctx, "rm -f "+shellQuote(configTemp), nil)
		return err
	}
	commit := fmt.Sprintf("set -eu; chmod 600 %s %s; mv -f %s %s; mv -f %s %s",
		shellQuote(configTemp), shellQuote(authTemp), shellQuote(configTemp), shellQuote(configPath), shellQuote(authTemp), shellQuote(authPath))
	if _, err := r.connection.Run(ctx, commit, nil); err != nil {
		_, _ = r.connection.Run(ctx, "rm -f "+shellQuote(configTemp)+" "+shellQuote(authTemp), nil)
		return codedError("remote_command_failed", err)
	}
	return nil
}

func (r *sshRemoteOperations) writeTemp(ctx context.Context, filename string, content []byte) error {
	if _, err := r.connection.Run(ctx, "umask 077; cat > "+shellQuote(filename), content); err != nil {
		return codedError("remote_command_failed", err)
	}
	return nil
}

func (r *sshRemoteOperations) Restore(ctx context.Context, codexDir string) error {
	configPath, authPath := path.Join(codexDir, "config.toml"), path.Join(codexDir, "auth.json")
	command := fmt.Sprintf(`set -eu
for file in %s %s; do
  if [ -e "$file.sub2api-bak" ]; then mv -f "$file.sub2api-bak" "$file"; chmod 600 "$file";
  elif [ -e "$file.sub2api-absent" ]; then rm -f "$file" "$file.sub2api-absent";
  fi
done`, shellQuote(configPath), shellQuote(authPath))
	if _, err := r.connection.Run(ctx, command, nil); err != nil {
		return codedError("remote_command_failed", err)
	}
	return nil
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `'"'"'`) + "'"
}

func randomSuffix() string {
	value := make([]byte, 8)
	if _, err := rand.Read(value); err != nil {
		return "fallback"
	}
	return hex.EncodeToString(value)
}
