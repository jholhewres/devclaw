// Package commands – setup_skills.go provides the interactive skill selection
// for the setup wizard. Uses the shared defaults from pkg/goclaw/skills.
package commands

import (
	"fmt"

	"github.com/jholhewres/goclaw/pkg/goclaw/skills"
)

// installEmbeddedSkills copies selected default skill templates to the skills directory.
func installEmbeddedSkills(selectedNames []string) {
	if len(selectedNames) == 0 {
		return
	}

	fmt.Println()
	fmt.Println("  Installing selected skills...")

	installed, skipped, failed := skills.InstallDefaultSkills("./skills", selectedNames)
	total := installed + skipped + failed

	if failed > 0 {
		fmt.Printf("  %d/%d installed, %d skipped, %d failed.\n", installed, total, skipped, failed)
	} else if skipped > 0 {
		fmt.Printf("  %d/%d installed, %d already existed.\n", installed, total, skipped)
	} else {
		fmt.Printf("  %d/%d skill(s) installed.\n", installed, total)
	}
}
