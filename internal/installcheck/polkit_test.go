package installcheck

import (
	"encoding/xml"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/frostyard/chairlift/internal/bootc"
	"github.com/frostyard/chairlift/internal/sysupdate"
	"github.com/frostyard/chairlift/internal/updex"
	"github.com/frostyard/chairlift/internal/updexhelper"
)

const (
	execPathAnnotation  = "org.freedesktop.policykit.exec.path"
	execArgv1Annotation = "org.freedesktop.policykit.exec.argv1"
)

type policyDocument struct {
	XMLName xml.Name       `xml:"policyconfig"`
	Actions []policyAction `xml:"action"`
}

type policyAction struct {
	ID          string             `xml:"id,attr"`
	Description string             `xml:"description"`
	Message     string             `xml:"message"`
	Defaults    policyDefaults     `xml:"defaults"`
	Annotations []policyAnnotation `xml:"annotate"`
}

type policyDefaults struct {
	AllowAny      string `xml:"allow_any"`
	AllowInactive string `xml:"allow_inactive"`
	AllowActive   string `xml:"allow_active"`
}

type policyAnnotation struct {
	Key   string `xml:"key,attr"`
	Value string `xml:",chardata"`
}

type expectedPolicyAction struct {
	ID          string
	Description string
	Message     string
	Path        string
	Argv1       string
}

func loadPolicy(t *testing.T, name string) policyDocument {
	t.Helper()

	path := filepath.Join(RepoRoot(), "data", name)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}

	var document policyDocument
	if err := xml.Unmarshal(data, &document); err != nil {
		t.Fatalf("parsing PolicyKit XML %s: %v", path, err)
	}
	if document.XMLName.Local != "policyconfig" {
		t.Fatalf("%s root element = %q, want policyconfig", path, document.XMLName.Local)
	}
	return document
}

func assertPolicyActions(t *testing.T, name string, expected []expectedPolicyAction) {
	t.Helper()

	document := loadPolicy(t, name)
	if len(document.Actions) != len(expected) {
		t.Fatalf("%s has %d actions, want %d", name, len(document.Actions), len(expected))
	}

	byID := make(map[string]policyAction, len(document.Actions))
	for _, action := range document.Actions {
		if _, exists := byID[action.ID]; exists {
			t.Fatalf("%s repeats action id %q", name, action.ID)
		}
		byID[action.ID] = action
	}

	wantDefaults := policyDefaults{
		AllowAny:      "auth_admin",
		AllowInactive: "auth_admin",
		AllowActive:   "auth_admin_keep",
	}
	for _, want := range expected {
		action, ok := byID[want.ID]
		if !ok {
			t.Errorf("%s is missing action %q", name, want.ID)
			continue
		}
		if action.Description != want.Description {
			t.Errorf("%s action %q description = %q, want %q", name, want.ID, action.Description, want.Description)
		}
		if action.Message != want.Message {
			t.Errorf("%s action %q message = %q, want %q", name, want.ID, action.Message, want.Message)
		}
		if !reflect.DeepEqual(action.Defaults, wantDefaults) {
			t.Errorf("%s action %q defaults = %+v, want %+v", name, want.ID, action.Defaults, wantDefaults)
		}

		annotations := make(map[string]string, len(action.Annotations))
		for _, annotation := range action.Annotations {
			if _, exists := annotations[annotation.Key]; exists {
				t.Errorf("%s action %q repeats annotation %q", name, want.ID, annotation.Key)
			}
			annotations[annotation.Key] = annotation.Value
		}
		wantAnnotations := map[string]string{execPathAnnotation: want.Path}
		if want.Argv1 != "" {
			wantAnnotations[execArgv1Annotation] = want.Argv1
		}
		if !reflect.DeepEqual(annotations, wantAnnotations) {
			t.Errorf("%s action %q annotations = %v, want %v", name, want.ID, annotations, wantAnnotations)
		}
	}
}

func TestPolkitPoliciesMatchPrivilegedHelpers(t *testing.T) {
	commands := updexhelper.SupportedCommands()
	expectedCommands := []string{
		updexhelper.CommandEnableFeature,
		updexhelper.CommandDisableFeature,
		updexhelper.CommandUpdate,
	}
	if !reflect.DeepEqual(commands, expectedCommands) {
		t.Fatalf("updexhelper.SupportedCommands() = %v, want %v", commands, expectedCommands)
	}

	assertPolicyActions(t, "org.frostyard.ChairLift.updex.policy", []expectedPolicyAction{
		{
			ID:          "org.frostyard.ChairLift.updex.enable-feature",
			Description: "Enable a system feature using updex",
			Message:     "Authentication is required to enable a system feature",
			Path:        updex.HelperPath,
			Argv1:       updexhelper.CommandEnableFeature,
		},
		{
			ID:          "org.frostyard.ChairLift.updex.disable-feature",
			Description: "Disable a system feature using updex",
			Message:     "Authentication is required to disable a system feature",
			Path:        updex.HelperPath,
			Argv1:       updexhelper.CommandDisableFeature,
		},
		{
			ID:          "org.frostyard.ChairLift.updex.update",
			Description: "Update enabled system features using updex",
			Message:     "Authentication is required to update enabled system features",
			Path:        updex.HelperPath,
			Argv1:       updexhelper.CommandUpdate,
		},
	})

	assertPolicyActions(t, "org.frostyard.ChairLift.bootc.policy", []expectedPolicyAction{
		{
			ID:          "org.frostyard.ChairLift.bootc.stage",
			Description: "Download and stage a system image update",
			Message:     "Authentication is required to stage a system update",
			Path:        bootc.StageScriptPath,
		},
	})

	assertPolicyActions(t, "org.frostyard.ChairLift.sysupdate.policy", []expectedPolicyAction{
		{
			ID:          "org.frostyard.ChairLift.sysupdate.stage",
			Description: "Download and stage a system image update",
			Message:     "Authentication is required to stage a system update",
			Path:        sysupdate.StageScriptPath,
		},
	})
}

func TestPolkitPasswordlessRulesAreAbsent(t *testing.T) {
	for _, name := range []string{
		"org.frostyard.ChairLift.updex.rules",
		"org.frostyard.ChairLift.bootc.rules",
		"org.frostyard.ChairLift.sysupdate.rules",
	} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(RepoRoot(), "data", name)
			if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
				t.Errorf("os.Stat(%q) error = %v, want file absent", path, err)
			}
		})
	}
}

func TestEveryUpdexCommandHasOnePolicyAction(t *testing.T) {
	document := loadPolicy(t, "org.frostyard.ChairLift.updex.policy")
	counts := make(map[string]int)
	for _, action := range document.Actions {
		for _, annotation := range action.Annotations {
			if annotation.Key == execArgv1Annotation {
				counts[annotation.Value]++
			}
		}
	}

	for _, command := range updexhelper.SupportedCommands() {
		if counts[command] != 1 {
			t.Errorf("PolicyKit actions selecting argv1=%q = %d, want exactly 1", command, counts[command])
		}
		delete(counts, command)
	}
	for command, count := range counts {
		t.Errorf("PolicyKit policy selects unsupported helper command %q in %d action(s)", command, count)
	}
	if t.Failed() {
		t.Logf("supported commands: %v", updexhelper.SupportedCommands())
	}
}
