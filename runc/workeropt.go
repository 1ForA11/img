package runc

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/boltdb/bolt"
	"github.com/containerd/containerd/content/local"
	"github.com/containerd/containerd/diff/apply"
	"github.com/containerd/containerd/diff/walking"
	ctdmetadata "github.com/containerd/containerd/metadata"
	ctdsnapshot "github.com/containerd/containerd/snapshots"
	"github.com/jessfraz/img/snapshots/fuse"
	"github.com/moby/buildkit/cache/metadata"
	containerdsnapshot "github.com/moby/buildkit/snapshot/containerd"
	"github.com/moby/buildkit/worker/base"
)

// NewWorkerOpt creates a WorkerOpt.
func NewWorkerOpt(root string) (opt base.WorkerOpt, err error) {
	name := "runc-fuse"

	// Create the root/
	root = filepath.Join(root, name)
	if err := os.MkdirAll(root, 0700); err != nil {
		return opt, err
	}

	// Create the metadata store.
	md, err := metadata.NewStore(filepath.Join(root, "metadata.db"))
	if err != nil {
		return opt, err
	}

	// Create the runc executor.
	exe, err := newExecutor(filepath.Join(root, "executor"))
	if err != nil {
		return opt, err
	}

	// Create the snapshotter.
	s, err := fuse.NewSnapshotter(filepath.Join(root, "snapshots"))
	if err != nil {
		return opt, fmt.Errorf("creating snapshotter failed: %v", err)
	}

	// Create the content store locally.
	c, err := local.NewStore(filepath.Join(root, "content"))
	if err != nil {
		return opt, err
	}

	// Open the bolt database for metadata.
	db, err := bolt.Open(filepath.Join(root, "containerdmeta.db"), 0644, nil)
	if err != nil {
		return opt, err
	}

	// Create the new database for metadata.
	mdb := ctdmetadata.NewDB(db, c, map[string]ctdsnapshot.Snapshotter{
		"fuse": s,
	})
	if err := mdb.Init(context.TODO()); err != nil {
		return opt, err
	}

	gc := func(ctx context.Context) error {
		_, err := mdb.GarbageCollect(ctx)
		return err
	}

	c = containerdsnapshot.NewContentStore(mdb.ContentStore(), "buildkit", gc)

	id, err := base.ID(root)
	if err != nil {
		return opt, err
	}

	xlabels := base.Labels("oci", "fuse")

	opt = base.WorkerOpt{
		ID:            id,
		Labels:        xlabels,
		MetadataStore: md,
		Executor:      exe,
		Snapshotter:   containerdsnapshot.NewSnapshotter(mdb.Snapshotter("fuse"), c, md, "buildkit", gc),
		ContentStore:  c,
		Applier:       apply.NewFileSystemApplier(c),
		Differ:        walking.NewWalkingDiff(c),
		ImageStore:    nil, // explicitly
	}
	return opt, nil
}
