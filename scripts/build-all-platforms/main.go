// Cross-compile the CLI with the dashboard assets already present in the checkout.
package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func main() {
	for _, targetOS := range []string{"windows", "linux", "darwin"} {
		for _, arch := range []string{"amd64", "arm64"} {
			name := "pinchtab"
			if targetOS == "windows" {
				name += ".exe"
			}
			output := filepath.Join("bin", targetOS+"-"+arch, name)
			if err := os.MkdirAll(filepath.Dir(output), 0755); err != nil {
				fail(err)
			}
			cmd := exec.Command("go", "build", "-trimpath", "-o", output, "./cmd/pinchtab")
			for _, env := range os.Environ() {
				key, _, _ := strings.Cut(env, "=")
				switch strings.ToUpper(key) {
				case "GOOS", "GOARCH", "CGO_ENABLED":
					continue
				}
				cmd.Env = append(cmd.Env, env)
			}
			cmd.Env = append(cmd.Env, "GOOS="+targetOS, "GOARCH="+arch, "CGO_ENABLED=0")
			cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
			fmt.Println("Building", targetOS+"/"+arch, "->", output)
			if err := cmd.Run(); err != nil {
				fail(err)
			}
			info, err := os.Stat(output)
			if err != nil {
				fail(err)
			}
			if info.Size() == 0 {
				fail(fmt.Errorf("empty executable: %s", output))
			}
		}
	}
	fmt.Println("Build complete: six executables in bin/")
}

func fail(err error) { fmt.Fprintln(os.Stderr, err); os.Exit(1) }
