// Copyright 2017 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package dep

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/golang/dep/test"
)

func TestTxnWriterBadInputs(t *testing.T) {
	h := test.NewHelper(t)
	defer h.Cleanup()

	h.TempDir("txnwriter")
	td := h.Path(".")

	var sw SafeWriter

	// no root
	if err := sw.WriteAllSafe(false); err == nil {
		t.Errorf("should have errored without a root path, but did not")
	}
	sw.Root = td

	if err := sw.WriteAllSafe(false); err != nil {
		t.Errorf("write with only root should be fine, just a no-op, but got err %q", err)
	}
	if err := sw.WriteAllSafe(true); err == nil {
		t.Errorf("should fail because no source manager was provided for writing vendor")
	}

	if err := sw.WriteAllSafe(true); err == nil {
		t.Errorf("should fail because no lock was provided from which to write vendor")
	}
	// now check dir validation
	sw.Root = filepath.Join(td, "nonexistent")
	if err := sw.WriteAllSafe(false); err == nil {
		t.Errorf("should have errored with nonexistent dir for root path, but did not")
	}

	sw.Root = filepath.Join(td, "myfile")
	srcf, err := os.Create(sw.Root)
	if err != nil {
		t.Fatal(err)
	}
	srcf.Close()
	if err := sw.WriteAllSafe(false); err == nil {
		t.Errorf("should have errored when root path is a file, but did not")
	}
}

func TestTxnWriter(t *testing.T) {
	test.NeedsExternalNetwork(t)
	test.NeedsGit(t)

	h := test.NewHelper(t)
	defer h.Cleanup()

	h.TempDir("")
	defer h.Cleanup()

	c := &Ctx{
		GOPATH: h.Path("."),
	}
	sm, err := c.SourceManager()
	defer sm.Release()
	h.Must(err)

	var sw SafeWriter
	var mpath, lpath, vpath string
	var count int
	reset := func() {
		pr := filepath.Join("src", "txnwriter"+strconv.Itoa(count))
		h.TempDir(pr)

		sw = SafeWriter{
			Root:          h.Path(pr),
			SourceManager: sm,
		}
		h.Cd(sw.Root)

		mpath = filepath.Join(sw.Root, ManifestName)
		lpath = filepath.Join(sw.Root, LockName)
		vpath = filepath.Join(sw.Root, "vendor")

		count++
	}
	reset()

	// super basic manifest and lock
	goldenm := "txn_writer/expected_manifest.json"
	goldenl := "txn_writer/expected_lock.json"
	wantm := h.GetTestFileString(goldenm)
	wantl := h.GetTestFileString(goldenl)

	m, err := readManifest(h.GetTestFile(goldenm))
	h.Must(err)
	l, err := readLock(h.GetTestFile(goldenl))
	h.Must(err)

	// Just write manifest
	sw.Manifest = m
	h.Must(sw.WriteAllSafe(false))
	h.MustExist(mpath)
	h.MustNotExist(lpath)
	h.MustNotExist(vpath)

	gotm := h.ReadManifest()
	if wantm != gotm {
		if *test.UpdateGolden {
			wantm = gotm
			if err = h.WriteTestFile(goldenm, gotm); err != nil {
				t.Fatal(err)
			}
		} else {
			t.Fatalf("expected %s, got %s", wantm, gotm)
		}
	}

	// Manifest and lock, but no vendor
	sw.Lock = l
	h.Must(sw.WriteAllSafe(false))
	h.MustExist(mpath)
	h.MustExist(lpath)
	h.MustNotExist(vpath)

	gotm = h.ReadManifest()
	if wantm != gotm {
		t.Fatalf("expected %s, got %s", wantm, gotm)
	}

	gotl := h.ReadLock()
	if wantl != gotl {
		if *test.UpdateGolden {
			wantl = gotl
			if err = h.WriteTestFile(goldenl, gotl); err != nil {
				t.Fatal(err)
			}
		} else {
			t.Fatalf("expected %s, got %s", wantl, gotl)
		}
	}

	h.Must(sw.WriteAllSafe(true))
	h.MustExist(mpath)
	h.MustExist(lpath)
	h.MustExist(vpath)
	h.MustExist(filepath.Join(vpath, "github.com", "sdboyer", "dep-test"))

	gotm = h.ReadManifest()
	if wantm != gotm {
		t.Fatalf("expected %s, got %s", wantm, gotm)
	}

	gotl = h.ReadLock()
	if wantl != gotl {
		t.Fatalf("expected %s, got %s", wantl, gotl)
	}

	// start fresh, ignoring the manifest now
	reset()
	sw.Lock = l
	sw.NewLock = l

	h.Must(sw.WriteAllSafe(false))
	// locks are equivalent, so nothing gets written
	h.MustNotExist(mpath)
	h.MustNotExist(lpath)
	h.MustNotExist(vpath)

	l2 := new(Lock)
	*l2 = *l
	// zero out the input hash to ensure non-equivalency
	l2.Memo = []byte{}
	sw.Lock = l2
	h.Must(sw.WriteAllSafe(true))
	h.MustNotExist(mpath)
	h.MustExist(lpath)
	h.MustExist(vpath)
	h.MustExist(filepath.Join(vpath, "github.com", "sdboyer", "dep-test"))

	gotl = h.ReadLock()
	if wantl != gotl {
		t.Fatalf("expected %s, got %s", wantl, gotl)
	}

	// repeat op to ensure good behavior when vendor dir already exists
	sw.Lock = nil
	h.Must(sw.WriteAllSafe(true))
	h.MustNotExist(mpath)
	h.MustExist(lpath)
	h.MustExist(vpath)
	h.MustExist(filepath.Join(vpath, "github.com", "sdboyer", "dep-test"))

	gotl = h.ReadLock()
	if wantl != gotl {
		t.Fatalf("expected %s, got %s", wantl, gotl)
	}

	// TODO test txn rollback cases. maybe we can force errors with chmodding?
}
