package docs

import (
	"os"
	"strings"
	"testing"
)

func TestREADMEValidationDocumentsCISmokeFastLaneScriptChecks(t *testing.T) {
	readme, err := os.ReadFile("../README.md")
	if err != nil {
		t.Fatalf("read README.md: %v", err)
	}
	workflow, err := os.ReadFile("../.github/workflows/ci-smoke.yml")
	if err != nil {
		t.Fatalf("read ci-smoke workflow: %v", err)
	}

	readmeText := string(readme)
	workflowText := string(workflow)
	for _, command := range []string{
		"bash -n scripts/smoke-local.sh",
		"bash -n scripts/smoke-docker.sh",
		"bash -n scripts/ci-smoke-runner-evidence.sh",
	} {
		if !strings.Contains(readmeText, command) {
			t.Fatalf("expected README validation docs to include %q", command)
		}
		if !strings.Contains(workflowText, "run: "+command) {
			t.Fatalf("expected CI Smoke workflow to include %q", command)
		}
	}

	for _, fragment := range []string{
		"CI Smoke mirrors those checks in the hosted fast lane",
		"operator smoke helpers",
		"The Docker smoke lane remains separate",
		"make smoke-docker",
	} {
		if !strings.Contains(readmeText, fragment) {
			t.Fatalf("expected README validation docs to include %q", fragment)
		}
	}
}
