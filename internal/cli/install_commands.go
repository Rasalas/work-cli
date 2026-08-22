package cli

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

const installerURL = "https://github.com/Rasalas/work-cli/releases/latest/download/install.sh"
const installerChecksumsURL = "https://github.com/Rasalas/work-cli/releases/latest/download/checksums.txt"

var runInstaller = runInstallerScript

func installCmd() *cobra.Command {
	var opts installOptions
	cmd := &cobra.Command{
		Use:   "install",
		Short: "Install work from the latest GitHub release",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runInstaller("install", opts.dir, opts.version)
		},
	}
	addInstallFlags(cmd, &opts)
	return cmd
}

func updateCmd() *cobra.Command {
	var opts installOptions
	cmd := &cobra.Command{
		Use:   "update",
		Short: "Update work from the latest GitHub release",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			dir := opts.dir
			if dir == "" {
				executable, err := os.Executable()
				if err != nil {
					return err
				}
				dir = filepath.Dir(executable)
			}
			return runInstaller("update", dir, opts.version)
		},
	}
	addInstallFlags(cmd, &opts)
	return cmd
}

func uninstallCmd() *cobra.Command {
	var opts installOptions
	cmd := &cobra.Command{
		Use:   "uninstall",
		Short: "Uninstall work",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			dir := opts.dir
			if dir == "" {
				executable, err := os.Executable()
				if err != nil {
					return err
				}
				dir = filepath.Dir(executable)
			}
			return runInstaller("uninstall", dir, "")
		},
	}
	cmd.Flags().StringVar(&opts.dir, "dir", "", "installation directory")
	return cmd
}

type installOptions struct {
	dir     string
	version string
}

func addInstallFlags(cmd *cobra.Command, opts *installOptions) {
	cmd.Flags().StringVar(&opts.dir, "dir", "", "installation directory")
	cmd.Flags().StringVar(&opts.version, "version", "", "release tag to install")
}

func runInstallerScript(action, dir, version string) error {
	script, err := downloadInstallerScript(installerURL)
	if err != nil {
		return err
	}
	defer os.Remove(script)

	checksums, err := downloadFile(installerChecksumsURL)
	if err != nil {
		_ = os.Remove(script)
		return fmt.Errorf("download installer checksums: %w", err)
	}
	if err := verifyChecksum(script, checksums, "install.sh"); err != nil {
		_ = os.Remove(script)
		return err
	}

	args := []string{script, action}
	if dir != "" {
		args = append(args, "--dir", dir)
	}
	if version != "" {
		args = append(args, "--version", version)
	}

	command := exec.Command("bash", args...)
	command.Stdout = out
	command.Stderr = os.Stderr
	command.Stdin = os.Stdin
	return command.Run()
}

func downloadFile(url string) ([]byte, error) {
	response, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode < 200 || response.StatusCode > 299 {
		return nil, fmt.Errorf("download %s: %s", url, response.Status)
	}
	return io.ReadAll(response.Body)
}

func downloadInstallerScript(url string) (string, error) {
	response, err := http.Get(url)
	if err != nil {
		return "", err
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode < 200 || response.StatusCode > 299 {
		return "", fmt.Errorf("download installer: %s", response.Status)
	}

	file, err := os.CreateTemp("", "work-install-*.sh")
	if err != nil {
		return "", err
	}
	defer func() { _ = file.Close() }()

	if _, err := file.ReadFrom(response.Body); err != nil {
		_ = os.Remove(file.Name())
		return "", err
	}
	if err := file.Chmod(0o700); err != nil {
		_ = os.Remove(file.Name())
		return "", err
	}
	return file.Name(), nil
}

// verifyChecksum checks that the SHA-256 of path matches the entry for
// name in checksums (the content of a release checksums.txt file).
func verifyChecksum(path string, checksums []byte, name string) error {
	expected, err := expectedChecksum(checksums, name)
	if err != nil {
		return err
	}
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()

	digest := sha256.New()
	if _, err := io.Copy(digest, file); err != nil {
		return err
	}
	actual := hex.EncodeToString(digest.Sum(nil))
	if actual != expected {
		return fmt.Errorf("checksum mismatch for %s: got %s, want %s", name, actual, expected)
	}
	return nil
}

func expectedChecksum(checksums []byte, name string) (string, error) {
	for _, line := range strings.Split(string(checksums), "\n") {
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) == 2 && filepath.Base(fields[1]) == name {
			return fields[0], nil
		}
	}
	return "", fmt.Errorf("checksum entry for %q not found", name)
}
