package i18n

import (
	"crypto/sha256"
	"encoding/hex"
	"slices"
	"testing"
)

// firstSetupSentences holds the SHA-256 of each sentence-length msgid that
// ChairLift once ported from First Setup (frostyard/first-setup b5f7de3).
// Those sentences are GPL-3.0 and ChairLift is MIT, so this file holds only
// their hashes (ADR-0018). Single words and short labels are common UI words
// and are not listed.
var firstSetupSentences = []string{
	"012834eb44e6c547ea6a30ade23e0272109bcf018b893682660fdc0ba9d82753",
	"0f89403b4fc2163caaff219d995ac968ed5c2022da1b814477fa0e8f6ebcb9e0",
	"114eb3e2904aae4cf7eaf90ab3661a17b4e4203503ced186d48f5665711ef00f",
	"2c33be8e78711643545484003be44fa006c62eb7950b72b559196264dbc70575",
	"4ce9b489aa8efd5b4c0fef374db0940e72cabe7e41aad4b590955c4d598a9c34",
	"69d7e6736566bbfe41e3bc7e232ffda9a29c52ec33cc198eb39d5ca779be8398",
	"875536aff7f9f9952a20bf7f438aec3ed3acda83eebb882d319cc88bd3680001",
	"915241cd8a9e5f79e5d023e5128baac40c8cba9737c6e79ca24dfb6cd530aff1",
	"a5e3a37dd0abf5aa9ef692d07054d122f81aa2b613f84a13bee659193e80691e",
	"d548fb1e8d501a05a840f92c83eefb0917f663097f8f3dad78e919d9ce74e87a",
	"d8fa75dfa3c0a4de75fa9a30c0418b50354096edf0522f4d70c0c8287952a7ca",
	"de1f6c329f5d17005955cfb5417342384fff3a61aef12d0eb96f2eb39bc59722",
	"f74edeb631565b81fa09571d36a9be62a3486e3e69183c93e61659fd1ab97a08",
}

// hashedIn returns the msgids whose SHA-256 is in hashes.
func hashedIn(msgids, hashes []string) []string {
	var found []string
	for _, msgid := range msgids {
		sum := sha256.Sum256([]byte(msgid))
		if slices.Contains(hashes, hex.EncodeToString(sum[:])) {
			found = append(found, msgid)
		}
	}
	return found
}

// TestNoMsgidCopiesFirstSetup fails when a first-run msgid is one of First
// Setup's sentences.
func TestNoMsgidCopiesFirstSetup(t *testing.T) {
	if len(Messages) == 0 {
		t.Fatal("Messages is empty, so the check compares nothing")
	}
	for _, msgid := range hashedIn(Messages, firstSetupSentences) {
		t.Errorf("msgid %q is a sentence of First Setup; write it in ChairLift's own words (ADR-0018)", msgid)
	}
}

// TestHashedInFindsOnlyAnExactMatch proves the comparison can fail.
func TestHashedInFindsOnlyAnExactMatch(t *testing.T) {
	sum := sha256.Sum256([]byte("a listed sentence"))
	hashes := []string{hex.EncodeToString(sum[:])}
	got := hashedIn([]string{"another sentence", "a listed sentence", "A listed sentence"}, hashes)
	if want := []string{"a listed sentence"}; !slices.Equal(got, want) {
		t.Errorf("hashedIn = %q, want %q", got, want)
	}
}
