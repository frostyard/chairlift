package flatpak

import (
	"slices"
	"testing"
)

func TestUpdateListArgs(t *testing.T) {
	tests := []struct {
		name        string
		user        bool
		wantFlag    string
		notWantFlag string
	}{
		{name: "user installation", user: true, wantFlag: "--user", notWantFlag: "--system"},
		{name: "system installation", user: false, wantFlag: "--system", notWantFlag: "--user"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := updateListArgs(tt.user)

			for _, want := range []string{"remote-ls", "--updates", "--app", tt.wantFlag} {
				if !slices.Contains(args, want) {
					t.Errorf("updateListArgs(%v) = %v, missing %q", tt.user, args, want)
				}
			}
			if slices.Contains(args, tt.notWantFlag) {
				t.Errorf("updateListArgs(%v) = %v, must not contain %q", tt.user, args, tt.notWantFlag)
			}
		})
	}
}

func TestParseUpdateList(t *testing.T) {
	tests := []struct {
		name   string
		output string
		user   bool
		want   []UpdateInfo
	}{
		{
			name:   "tab separated rows",
			output: "Firefox\torg.mozilla.firefox\t120.0\tstable\tflathub\nGIMP\torg.gimp.GIMP\t2.10.36\tstable\tflathub\n",
			user:   false,
			want: []UpdateInfo{
				{Name: "Firefox", ApplicationID: "org.mozilla.firefox", NewVersion: "120.0", Branch: "stable", Origin: "flathub", Installation: "system"},
				{Name: "GIMP", ApplicationID: "org.gimp.GIMP", NewVersion: "2.10.36", Branch: "stable", Origin: "flathub", Installation: "system"},
			},
		},
		{
			name:   "whitespace separated fallback",
			output: "Firefox   org.mozilla.firefox   120.0   stable   flathub",
			user:   false,
			want: []UpdateInfo{
				{Name: "Firefox", ApplicationID: "org.mozilla.firefox", NewVersion: "120.0", Branch: "stable", Origin: "flathub", Installation: "system"},
			},
		},
		{
			name:   "short row is partially parsed",
			output: "Firefox org.mozilla.firefox 120.0",
			user:   false,
			want: []UpdateInfo{
				{Name: "Firefox", ApplicationID: "org.mozilla.firefox", NewVersion: "120.0", Installation: "system"},
			},
		},
		{
			name:   "row with fewer than two fields is skipped",
			output: "Firefox\nGIMP\torg.gimp.GIMP\t2.10.36\tstable\tflathub",
			user:   false,
			want: []UpdateInfo{
				{Name: "GIMP", ApplicationID: "org.gimp.GIMP", NewVersion: "2.10.36", Branch: "stable", Origin: "flathub", Installation: "system"},
			},
		},
		{
			name:   "blank and whitespace-only lines are skipped",
			output: "\n   \nFirefox\torg.mozilla.firefox\t120.0\tstable\tflathub\n\t\n",
			user:   false,
			want: []UpdateInfo{
				{Name: "Firefox", ApplicationID: "org.mozilla.firefox", NewVersion: "120.0", Branch: "stable", Origin: "flathub", Installation: "system"},
			},
		},
		{
			name:   "empty output yields no updates",
			output: "",
			user:   false,
			want:   nil,
		},
		{
			name:   "user installation label",
			output: "Firefox\torg.mozilla.firefox\t120.0\tstable\tflathub",
			user:   true,
			want: []UpdateInfo{
				{Name: "Firefox", ApplicationID: "org.mozilla.firefox", NewVersion: "120.0", Branch: "stable", Origin: "flathub", Installation: "user"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseUpdateList(tt.output, tt.user)
			if err != nil {
				t.Fatalf("parseUpdateList() unexpected error: %v", err)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("parseUpdateList() = %+v, want %+v", got, tt.want)
			}
			for i := range tt.want {
				if got[i] != tt.want[i] {
					t.Errorf("parseUpdateList()[%d] = %+v, want %+v", i, got[i], tt.want[i])
				}
			}
		})
	}
}
