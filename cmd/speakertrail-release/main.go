// Command speakertrail-release makes the update key, and signs and lists the
// builds the app updates itself to. It is not shipped. Tim runs it once, to
// make the key, and the publish workflow runs the rest. See docs/UPDATES.md.
//
//	speakertrail-release key
//	    make an update key: the public half into the repository, the
//	    private half to the clipboard, for GitHub's secrets and the
//	    password manager
//	speakertrail-release check -zip F
//	    say whether a build could read input meant for other apps, watch
//	    the screen or the camera, or ask for administrator rights, see
//	    inputs.go. It needs no key, so it runs before the key is near.
//	speakertrail-release pack -zip F -out D [-list F -assets F] [-from URL]
//	    write the build again as the zip it is published as, its files
//	    stored, with its parts, a piece for every file, a patch of every
//	    file that changed against the builds an app may run, read from the
//	    release, and the zip packed with zstd, see pack.go, patches.go and
//	    release.go
//	speakertrail-release sign -zip F -channel C -name N -version V -commit X -url U -out F [-parts F]
//	    sign a build with the private half, read from SPEAKERTRAIL_UPDATE_KEY,
//	    and write its entry for the channel list, unless the build could
//	    read input meant for other apps, watch the screen or the camera,
//	    or ask for administrator rights, see inputs.go. With the parts
//	    pack wrote, it signs the stored zip beside them too.
//	speakertrail-release list -out F [-newest CHANNEL=COMMIT]... [-conflict CHANNEL]... ENTRY...
//	    write the channel list from the entries of every channel, with the
//	    newest commit of each channel that has one still to be built, and
//	    the pull requests that no longer merge into main
//	speakertrail-release unreachable -list F -assets F [-from URL]
//	    name the files of the release that no build an app may run
//	    reaches any more, one a line, for the workflow to remove, see
//	    release.go
package main

import (
	"archive/zip"
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/klauspost/compress/zstd"

	"github.com/timmrkz/speakertrail/internal/updates"
)

// publicKeyFile is where the public half lives, beside the program that
// builds it in.
const publicKeyFile = "cmd/speakertrail-app/update-key.txt"

func main() {
	if len(os.Args) < 2 {
		usage()
	}
	var err error
	switch os.Args[1] {
	case "key":
		err = makeKey()
	case "pack":
		err = packCommand(os.Args[2:])
	case "check":
		err = check(os.Args[2:])
	case "sign":
		err = sign(os.Args[2:])
	case "list":
		err = list(os.Args[2:])
	case "unreachable":
		err = unreachableCommand(os.Args[2:])
	default:
		usage()
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "speakertrail-release:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: speakertrail-release key | pack -zip FILE -out DIR [-list FILE -assets FILE] | check -zip FILE | sign ... | list -out FILE ENTRY... | unreachable -list FILE -assets FILE")
	os.Exit(2)
}

// makeKey makes the pair. The private half never touches a file: it goes to
// the clipboard, from where it is pasted into GitHub's secrets and the
// password manager, and the clipboard is emptied by the next copy.
func makeKey() error {
	if text, _ := os.ReadFile(publicKeyFile); strings.TrimSpace(string(text)) != "" {
		return fmt.Errorf("%s already holds a key. Every build made with it trusts only that key, so a new one is a decision, not a command: empty the file first if it really is meant", publicKeyFile)
	}
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return err
	}
	pub := base64.StdEncoding.EncodeToString(public)
	priv := base64.StdEncoding.EncodeToString(private.Seed())
	if err := os.WriteFile(publicKeyFile, []byte(pub+"\n"), 0o644); err != nil {
		return err
	}
	fmt.Printf("The public half is in %s:\n\n  %s\n\n", publicKeyFile, pub)
	copy := exec.Command("pbcopy")
	copy.Stdin = strings.NewReader(priv)
	if copy.Run() == nil {
		fmt.Println("The private half is on the clipboard. Paste it into")
		fmt.Println("  1. GitHub, the repository's Settings, Environments, updates,")
		fmt.Println("     Add environment secret, named SPEAKERTRAIL_UPDATE_KEY")
		fmt.Println("  2. your password manager")
		fmt.Println("and then copy something else, so it leaves the clipboard.")
		return nil
	}
	fmt.Println("There is no clipboard here, so this is the private half. Put it in")
	fmt.Println("the secrets of GitHub's environment updates as SPEAKERTRAIL_UPDATE_KEY and")
	fmt.Println("in your password manager,")
	fmt.Println("and nowhere else:")
	fmt.Printf("\n  %s\n", priv)
	return nil
}

// keys reads both halves and refuses a pair that does not belong together:
// a build signed with a key the app does not carry is a build nobody can
// install, and it is better found here than on the Mac.
func keys() (ed25519.PrivateKey, ed25519.PublicKey, error) {
	secret := os.Getenv("SPEAKERTRAIL_UPDATE_KEY")
	if secret == "" {
		return nil, nil, errors.New("SPEAKERTRAIL_UPDATE_KEY is not set")
	}
	private, err := updates.PrivateKey(secret)
	if err != nil {
		return nil, nil, err
	}
	text, err := os.ReadFile(publicKeyFile)
	if err != nil {
		return nil, nil, err
	}
	public, err := updates.PublicKey(string(text))
	if err != nil {
		return nil, nil, err
	}
	if public == nil {
		return nil, nil, fmt.Errorf("%s is empty, so no app would trust the build", publicKeyFile)
	}
	if !public.Equal(private.Public()) {
		return nil, nil, fmt.Errorf("SPEAKERTRAIL_UPDATE_KEY is not the private half of %s", publicKeyFile)
	}
	return private, public, nil
}

// check reads the build the way sign does, without the key. publish.yml
// runs it in a step of its own before the step that has the key, so what
// is read from the zip is read where no secret is.
func check(args []string) error {
	fs := flag.NewFlagSet("check", flag.ExitOnError)
	zipPath := fs.String("zip", "", "the zip holding the app")
	_ = fs.Parse(args)
	return checkInputs(*zipPath)
}

// sign checks the build again before it signs it, so a sign run on its
// own never signs what check would refuse.
func sign(args []string) error {
	fs := flag.NewFlagSet("sign", flag.ExitOnError)
	zipPath := fs.String("zip", "", "the zip holding the app")
	channel := fs.String("channel", "", "main, or pr- and the pull request's number")
	name := fs.String("name", "", "what the app shows for the channel")
	version := fs.String("version", "", "the build's version")
	commit := fs.String("commit", "", "the commit it was built from")
	where := fs.String("url", "", "where the zip will be downloaded from")
	out := fs.String("out", "", "where the entry goes")
	parts := fs.String("parts", "", "the parts pack wrote, beside the stored zip and the zip packed, if it was packed")
	_ = fs.Parse(args)
	return signBuild(*zipPath, *channel, *name, *version, *commit, *where, *parts, *out)
}

// signBuild signs the zip and writes its entry. With parts, it signs the
// stored zip beside them too, once they are read as its parts, the zip
// packed beside them unpacks to it and it holds the same files as the zip
// check read, and the entry names it. An app that knows parts installs
// that zip, and an app from before installs the zip.
func signBuild(zipPath, channel, name, version, commit, where, parts, out string) error {
	if err := checkInputs(zipPath); err != nil {
		return err
	}
	private, public, err := keys()
	if err != nil {
		return err
	}
	f, err := os.Open(zipPath)
	if err != nil {
		return err
	}
	digest, size, err := updates.Digest(f)
	f.Close()
	if err != nil {
		return err
	}
	// A pull request's title can be long, and the app shows it in a list.
	shown := []rune(strings.TrimSpace(name))
	if len(shown) > 100 {
		shown = append(shown[:99], '…')
	}
	b := updates.Build{
		Channel: channel, Name: string(shown), Version: version, Commit: commit, URL: where,
		Size: size, SHA256: hex.EncodeToString(digest), Signature: updates.Sign(private, digest),
		Published: time.Now().UTC().Truncate(time.Second),
	}
	b.Claim = updates.SignClaim(private, b)
	if parts != "" {
		if err := sameApp(zipPath, filepath.Join(filepath.Dir(parts), storedZip)); err != nil {
			return err
		}
		st, err := signStored(private, b, filepath.Dir(parts), parts)
		if err != nil {
			return err
		}
		b.Stored = &st
	}
	if err := b.Check(); err != nil {
		return err
	}
	if err := updates.Verify(public, b); err != nil {
		return err
	}
	if err := updates.VerifyClaim(public, b); err != nil {
		return err
	}
	if st, ok := b.AsStored(); ok {
		if err := updates.Verify(public, st); err != nil {
			return err
		}
		if err := updates.VerifyClaim(public, st); err != nil {
			return err
		}
	}
	return writeJSON(out, b)
}

// unpacksTo says whether a zip packed with zstd unpacks to the zip with
// this SHA-256.
func unpacksTo(packed, digest []byte) error {
	dec, err := zstd.NewReader(bytes.NewReader(packed))
	if err != nil {
		return err
	}
	defer dec.Close()
	h := sha256.New()
	if _, err := io.Copy(h, dec); err != nil {
		return fmt.Errorf("the packed zip does not unpack: %w", err)
	}
	if !bytes.Equal(h.Sum(nil), digest) {
		return errors.New("the packed zip is of another zip")
	}
	return nil
}

// sameApp says whether two zips hold the same files, by name, size, mode
// and the SHA-256 of what each holds, so the stored zip signed beside a
// zip is the app check passed, byte for byte.
func sameApp(a, b string) error {
	za, err := zip.OpenReader(a)
	if err != nil {
		return err
	}
	defer za.Close()
	zb, err := zip.OpenReader(b)
	if err != nil {
		return err
	}
	defer zb.Close()
	if len(za.File) != len(zb.File) {
		return errors.New("the stored zip holds other files than the zip")
	}
	for i, f := range za.File {
		g := zb.File[i]
		if f.Name != g.Name || f.UncompressedSize64 != g.UncompressedSize64 || f.Mode() != g.Mode() {
			return fmt.Errorf("the stored zip holds %s otherwise than the zip", f.Name)
		}
		fs, err := fileSum(f)
		if err != nil {
			return err
		}
		gs, err := fileSum(g)
		if err != nil {
			return err
		}
		if !bytes.Equal(fs, gs) {
			return fmt.Errorf("the stored zip holds other bytes in %s than the zip", f.Name)
		}
	}
	return nil
}

// fileSum is the SHA-256 of what a file of a zip holds.
func fileSum(f *zip.File) ([]byte, error) {
	r, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer r.Close()
	h := sha256.New()
	if _, err := io.Copy(h, r); err != nil {
		return nil, err
	}
	return h.Sum(nil), nil
}

// signStored signs the stored zip pack left in packed as the build b is,
// and checks that the parts and the zip packed beside it make that zip.
func signStored(private ed25519.PrivateKey, b updates.Build, packed, parts string) (updates.Stored, error) {
	f, err := os.Open(filepath.Join(packed, storedZip))
	if err != nil {
		return updates.Stored{}, err
	}
	digest, size, err := updates.Digest(f)
	f.Close()
	if err != nil {
		return updates.Stored{}, err
	}
	sum := hex.EncodeToString(digest)
	data, err := os.ReadFile(parts)
	if err != nil {
		return updates.Stored{}, err
	}
	if _, err := updates.ParseParts(data, size, sum); err != nil {
		return updates.Stored{}, err
	}
	whole, err := os.ReadFile(filepath.Join(packed, packedWhole))
	if err != nil {
		return updates.Stored{}, err
	}
	if err := unpacksTo(whole, digest); err != nil {
		return updates.Stored{}, err
	}
	b.Size, b.SHA256 = size, sum
	return updates.Stored{
		Size: size, SHA256: sum, Signature: updates.Sign(private, digest),
		Claim: updates.SignClaim(private, b), PackedSize: int64(len(whole)),
	}, nil
}

// list writes the channel list from every channel's entry. Each entry is
// read with the same rules as the app reads the list, and checked against
// the public key, so the list never promises a build no app will take.
func list(args []string) error {
	fs := flag.NewFlagSet("list", flag.ExitOnError)
	out := fs.String("out", "", "where the channel list goes")
	// A channel's newest commit, when it has one still to be built. The
	// workflow knows which pull requests have one, and the entry, written
	// when the build was made, cannot.
	newest := map[string]string{}
	fs.Func("newest", "a channel's newest commit still to be built, as CHANNEL=COMMIT", func(v string) error {
		channel, commit, ok := strings.Cut(v, "=")
		if !ok || !updates.ValidChannel(channel) {
			return fmt.Errorf("%q is not a channel and a commit", v)
		}
		newest[channel] = commit
		return nil
	})
	// The pull requests that no longer merge into main. Like the newest
	// commit, the workflow knows it when it writes the list, and the
	// entry cannot.
	conflict := map[string]bool{}
	fs.Func("conflict", "a pull request that no longer merges into main, as its channel", func(v string) error {
		if !updates.ValidChannel(v) || v == "main" {
			return fmt.Errorf("%q is not the channel of a pull request", v)
		}
		conflict[v] = true
		return nil
	})
	_ = fs.Parse(args)
	text, err := os.ReadFile(publicKeyFile)
	if err != nil {
		return err
	}
	public, err := updates.PublicKey(string(text))
	if err != nil || public == nil {
		return fmt.Errorf("no public key in %s", publicKeyFile)
	}
	var l updates.List
	for _, path := range fs.Args() {
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		var b updates.Build
		if err := json.Unmarshal(data, &b); err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		if err := b.Check(); err != nil {
			fmt.Fprintf(os.Stderr, "left out %s: %v\n", path, err)
			continue
		}
		if err := updates.Verify(public, b); err != nil {
			fmt.Fprintf(os.Stderr, "left out %s: %v\n", path, err)
			continue
		}
		// An entry from before claims were made is kept, for the apps
		// that do not check them yet, and the apps that do leave it alone.
		// One whose claim is there and wrong is no build of ours.
		if b.Claim != "" {
			if err := updates.VerifyClaim(public, b); err != nil {
				fmt.Fprintf(os.Stderr, "left out %s: %v\n", path, err)
				continue
			}
		}
		// The stored zip is what an app that knows parts installs, so its
		// signature and claim are held the same way.
		if st, ok := b.AsStored(); ok {
			if err := errors.Join(updates.Verify(public, st), updates.VerifyClaim(public, st)); err != nil {
				fmt.Fprintf(os.Stderr, "left out %s: %v\n", path, err)
				continue
			}
		}
		b.Newest = newest[b.Channel]
		b.Conflict = conflict[b.Channel]
		l.Channels = append(l.Channels, b)
	}
	// When it was written, so the app can tell the newer of the list's
	// two copies, see updates.ListCopies.
	l.Written = time.Now().UTC().Truncate(time.Second)
	data, err := json.Marshal(l)
	if err != nil {
		return err
	}
	// The same reading the app does, so what is written is what it keeps,
	// and a newest commit that is not one is not written.
	parsed, err := updates.Parse(data)
	if err != nil {
		return err
	}
	return writeJSON(*out, parsed)
}

func writeJSON(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}
