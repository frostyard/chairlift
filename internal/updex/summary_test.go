package updex

import (
	"reflect"
	"testing"
)

func TestSummarizeUpdate(t *testing.T) {
	tr := func(mods ...func(*TransferUpdate)) TransferUpdate {
		u := TransferUpdate{Component: "comp"}
		for _, m := range mods {
			m(&u)
		}
		return u
	}
	feature := func(name string, transfers ...TransferUpdate) FeatureUpdate {
		return FeatureUpdate{Feature: name, Results: transfers}
	}

	tests := []struct {
		name string
		in   FeatureUpdate
		want FeatureOutcome
	}{
		{name: "no transfers is current", in: feature("f"), want: FeatureOutcome{Feature: "f", Class: ClassCurrent}},
		{
			name: "installed and unchanged is current",
			in:   feature("f", tr(func(u *TransferUpdate) { u.Installed = true })),
			want: FeatureOutcome{Feature: "f", Class: ClassCurrent},
		},
		{
			name: "downloaded is updated",
			in:   feature("f", tr(func(u *TransferUpdate) { u.Downloaded = true })),
			want: FeatureOutcome{Feature: "f", Class: ClassUpdated},
		},
		{
			name: "relinked is updated",
			in:   feature("f", tr(func(u *TransferUpdate) { u.Linked = true })),
			want: FeatureOutcome{Feature: "f", Class: ClassUpdated},
		},
		{
			name: "held keeps the first message",
			in: feature("f",
				tr(func(u *TransferUpdate) { u.Held = true; u.NextActionMessage = "held at 1 by rollback of 2" }),
				tr(func(u *TransferUpdate) { u.Held = true; u.NextActionMessage = "second" })),
			want: FeatureOutcome{Feature: "f", Class: ClassHeld, Reason: "held at 1 by rollback of 2"},
		},
		{
			name: "needs image lists every component",
			in: feature("f",
				tr(func(u *TransferUpdate) { u.Component = "a"; u.NeedsImage = "needs image >= 5" }),
				tr(func(u *TransferUpdate) {
					u.Component = "b"
					u.NotLinked = true
					u.NextActionMessage = "not linked: a needs image >= 5"
				}),
				tr(func(u *TransferUpdate) { u.Component = "c"; u.NeedsImage = "needs image >= 7" })),
			want: FeatureOutcome{
				Feature: "f", Class: ClassWaitingForImage,
				Reason: "a: needs image >= 5, c: needs image >= 7",
			},
		},
		{
			name: "not linked only strips the prefix of the first message",
			in: feature("f",
				tr(func(u *TransferUpdate) { u.NotLinked = true; u.NextActionMessage = "not linked: a needs image >= 5" }),
				tr(func(u *TransferUpdate) { u.NotLinked = true; u.NextActionMessage = "not linked: other" })),
			want: FeatureOutcome{Feature: "f", Class: ClassWaitingForImage, Reason: "a needs image >= 5"},
		},
		{
			name: "failed keeps the first error",
			in: feature("f",
				tr(func(u *TransferUpdate) { u.Error = "download failed" }),
				tr(func(u *TransferUpdate) { u.Error = "second" })),
			want: FeatureOutcome{Feature: "f", Class: ClassFailed, Reason: "download failed"},
		},
		{
			name: "failed beats waiting for image (not-linked sibling of a failure)",
			in: feature("f",
				tr(func(u *TransferUpdate) { u.NeedsImage = "needs image >= 5" }),
				tr(func(u *TransferUpdate) { u.Error = "boom" })),
			want: FeatureOutcome{Feature: "f", Class: ClassFailed, Reason: "boom"},
		},
		{
			name: "waiting for image beats held",
			in: feature("f",
				tr(func(u *TransferUpdate) { u.Held = true; u.NextActionMessage = "held" }),
				tr(func(u *TransferUpdate) { u.NeedsImage = "needs image >= 5" })),
			want: FeatureOutcome{Feature: "f", Class: ClassWaitingForImage, Reason: "comp: needs image >= 5"},
		},
		{
			name: "held beats updated",
			in: feature("f",
				tr(func(u *TransferUpdate) { u.Downloaded = true }),
				tr(func(u *TransferUpdate) { u.Held = true; u.NextActionMessage = "held" })),
			want: FeatureOutcome{Feature: "f", Class: ClassHeld, Reason: "held"},
		},
		{
			name: "waiting for image beats updated",
			in: feature("f",
				tr(func(u *TransferUpdate) { u.Downloaded = true; u.Linked = true }),
				tr(func(u *TransferUpdate) { u.NotLinked = true; u.NextActionMessage = "not linked: x" })),
			want: FeatureOutcome{Feature: "f", Class: ClassWaitingForImage, Reason: "x"},
		},
		{
			name: "failed beats updated",
			in: feature("f",
				tr(func(u *TransferUpdate) { u.Downloaded = true }),
				tr(func(u *TransferUpdate) { u.Error = "boom" })),
			want: FeatureOutcome{Feature: "f", Class: ClassFailed, Reason: "boom"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SummarizeUpdate([]FeatureUpdate{tt.in})
			if len(got.Features) != 1 || got.Features[0] != tt.want {
				t.Fatalf("SummarizeUpdate = %+v, want one outcome %+v", got.Features, tt.want)
			}
		})
	}
}

func TestSummarizeUpdateKeepsOrderRefreshSkippedAndFiltersByClass(t *testing.T) {
	results := []FeatureUpdate{
		{Feature: "a", Results: []TransferUpdate{{Downloaded: true}}, RefreshSkipped: "needs image >= 9 to refresh"},
		{Feature: "b", RefreshSkipped: "needs image >= 9 to refresh"},
		{Feature: "c", Results: []TransferUpdate{{Linked: true}}},
	}
	got := SummarizeUpdate(results)

	if got.RefreshSkipped != "needs image >= 9 to refresh" {
		t.Errorf("RefreshSkipped = %q", got.RefreshSkipped)
	}
	want := []FeatureOutcome{
		{Feature: "a", Class: ClassUpdated},
		{Feature: "b", Class: ClassCurrent},
		{Feature: "c", Class: ClassUpdated},
	}
	if !reflect.DeepEqual(got.Features, want) {
		t.Errorf("Features = %+v, want %+v", got.Features, want)
	}
	if updated := got.Of(ClassUpdated); len(updated) != 2 || updated[0].Feature != "a" || updated[1].Feature != "c" {
		t.Errorf("Of(ClassUpdated) = %+v, want a and c", updated)
	}
	if failed := got.Of(ClassFailed); len(failed) != 0 {
		t.Errorf("Of(ClassFailed) = %+v, want none", failed)
	}
}

func TestSummarizeUpdateEmpty(t *testing.T) {
	for _, in := range [][]FeatureUpdate{nil, {}} {
		got := SummarizeUpdate(in)
		if len(got.Features) != 0 || got.RefreshSkipped != "" {
			t.Errorf("SummarizeUpdate(%v) = %+v, want empty", in, got)
		}
	}
}

func TestUpdateClassString(t *testing.T) {
	for class, want := range map[UpdateClass]string{
		ClassCurrent: "current", ClassUpdated: "updated", ClassHeld: "held",
		ClassWaitingForImage: "waiting-for-image", ClassFailed: "failed", UpdateClass(99): "unknown",
	} {
		if got := class.String(); got != want {
			t.Errorf("UpdateClass(%d).String() = %q, want %q", int(class), got, want)
		}
	}
}
