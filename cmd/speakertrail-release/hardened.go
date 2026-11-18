package main

// A build Tim updates to is signed with his Developer ID and the hardened
// runtime, see scripts/sign-macos.sh and docs/UPDATES.md. Once
// build/team-id.txt names his team, publish.yml tells sign -team, and sign
// refuses a build that lost it on the way: a program signed ad hoc,
// without the hardened runtime, or by another team. Under the hardened
// runtime macOS loads only libraries of the app's own team, so a library
// of another would not even open.

import (
	"archive/zip"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"strings"
)

// The flags of a code directory that say how a program was signed.
const (
	flagAdhoc   = 0x2
	flagRuntime = 0x10000
)

// signing is how one program, or one architecture of it, was signed.
type signing struct {
	signed bool
	flags  uint32
	team   string
}

// signingOf reads the code directory codesign writes into slot 0 of the
// signature: its flags, at 12, and from version 0x20200 on the offset of
// its team's name, at 48, all big-endian.
func signingOf(a arch) (signing, error) {
	cd, err := signatureBlob(a.f, a.data, 0, 0xfade0c02)
	if err != nil || cd == nil {
		return signing{}, err
	}
	be := binary.BigEndian
	if len(cd) < 44 {
		return signing{}, errors.New("its code directory is too short")
	}
	s := signing{signed: true, flags: be.Uint32(cd[12:])}
	if be.Uint32(cd[8:]) >= 0x20200 && len(cd) >= 52 {
		if at := be.Uint32(cd[48:]); at != 0 {
			if uint64(at) >= uint64(len(cd)) {
				return signing{}, errors.New("its team is not where its code directory says")
			}
			name, _, found := strings.Cut(string(cd[at:]), "\x00")
			if !found {
				return signing{}, errors.New("its team's name has no end")
			}
			s.team = name
		}
	}
	return s, nil
}

// checkHardened reads every program of the zip and says each that is not
// signed with the hardened runtime by the team given.
func checkHardened(zipPath, team string) error {
	z, err := zip.OpenReader(zipPath)
	if err != nil {
		return fmt.Errorf("the build is not a zip: %w", err)
	}
	defer z.Close()
	var found []string
	programs := 0
	for _, f := range z.File {
		if f.FileInfo().IsDir() || f.UncompressedSize64 > maxUnpacked {
			continue
		}
		r, err := f.Open()
		if err != nil {
			return fmt.Errorf("%s: %w", f.Name, err)
		}
		data, err := io.ReadAll(io.LimitReader(r, int64(f.UncompressedSize64)+1))
		r.Close()
		if err != nil {
			return fmt.Errorf("%s: %w", f.Name, err)
		}
		arches, err := archesOf(data)
		if err != nil {
			found = append(found, f.Name+": "+err.Error())
			continue
		}
		for _, a := range arches {
			s, err := signingOf(a)
			switch {
			case err != nil:
				found = append(found, fmt.Sprintf("%s: its signature cannot be read: %v", f.Name, err))
			case !s.signed:
				found = append(found, f.Name+": is not signed")
			case s.flags&flagAdhoc != 0:
				found = append(found, f.Name+": is signed ad hoc")
			case s.flags&flagRuntime == 0:
				found = append(found, f.Name+": is signed without the hardened runtime")
			case s.team == "":
				found = append(found, f.Name+": is signed by no team")
			case s.team != team:
				found = append(found, f.Name+": is signed by team "+s.team)
			default:
				programs++
			}
		}
	}
	if programs == 0 && len(found) == 0 {
		found = append(found, "it holds no program")
	}
	if found = tidy(found); len(found) > 0 {
		return fmt.Errorf("the build is not signed with the Developer ID of team %s and the hardened runtime, so it is not signed for updates. See scripts/sign-macos.sh:\n  %s", team, strings.Join(found, "\n  "))
	}
	return nil
}
