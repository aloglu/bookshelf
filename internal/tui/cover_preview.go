package tui

import (
	"fmt"
	"os/exec"
)

func OfferCoverPreview(path string, folder bool) error {
	action := "Open preview"
	if folder {
		action = "Open covers folder"
	}
	for {
		choice, chosen, err := RunDecision(DecisionRequest{
			Title:       "Cover Saved",
			Description: fmt.Sprintf("Saved to:\n%s", path),
			Options: []DecisionOption{
				{ID: "preview", Label: action},
				{ID: "done", Label: "Done"},
			},
			Default: 1,
		})
		if err != nil || !chosen || choice == "done" {
			return err
		}
		if err := openFile(path); err != nil {
			return err
		}
	}
}

func openFile(path string) error {
	type candidate struct {
		name string
		args []string
	}
	for _, opener := range []candidate{
		{name: "xdg-open", args: []string{path}},
		{name: "gio", args: []string{"open", path}},
	} {
		commandPath, err := exec.LookPath(opener.name)
		if err != nil {
			continue
		}
		command := exec.Command(commandPath, opener.args...)
		if err := command.Start(); err != nil {
			continue
		}
		go func() { _ = command.Wait() }()
		return nil
	}
	return fmt.Errorf("no supported image viewer launcher was found; open %s manually", path)
}
