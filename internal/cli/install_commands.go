package cli

import (
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/spf13/cobra"
)

const installerURL = "https://github.com/Rasalas/work-cli/releases/latest/download/install.sh"

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

func downloadInstallerScript(url string) (string, error) {
	response, err := http.Get(url)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode > 299 {
		return "", fmt.Errorf("download installer: %s", response.Status)
	}

	file, err := os.CreateTemp("", "work-install-*.sh")
	if err != nil {
		return "", err
	}
	defer file.Close()

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
