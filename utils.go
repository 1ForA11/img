package main

import (
	"fmt"
	"path/filepath"

	"github.com/hanwen/go-fuse/fuse"
	"github.com/jessfraz/img/runc"
	"github.com/moby/buildkit/control"
	"github.com/moby/buildkit/frontend"
	"github.com/moby/buildkit/frontend/dockerfile"
	"github.com/moby/buildkit/worker"
)

func createController(cmd command) (*control.Controller, *fuse.Server, error) {
	// Create the runc worker.
	opt, fuseserver, err := runc.NewWorkerOpt(defaultStateDirectory)
	if err != nil {
		return nil, fuseserver, fmt.Errorf("creating runc worker opt failed: %v", err)
	}

	localDirs := getLocalDirs(cmd)

	w, err := runc.NewWorker(opt, localDirs)
	if err != nil {
		return nil, fuseserver, err
	}

	// Create the worker controller.
	wc := &worker.Controller{}
	if err = wc.Add(w); err != nil {
		return nil, fuseserver, err
	}

	// Add the frontends.
	frontends := map[string]frontend.Frontend{}
	frontends["dockerfile.v0"] = dockerfile.NewDockerfileFrontend()

	// Create the controller.
	controller, err := control.NewController(control.Opt{
		WorkerController: wc,
		Frontends:        frontends,
		CacheExporter:    w.CacheExporter,
		CacheImporter:    w.CacheImporter,
	})
	return controller, fuseserver, err
}

func getLocalDirs(c command) map[string]string {
	// only return the local dirs for buildCommand
	cmd, ok := c.(*buildCommand)
	if !ok {
		return nil
	}

	file := cmd.dockerfilePath
	if file == "" {
		file = filepath.Join(cmd.contextDir, "Dockerfile")
	}

	return map[string]string{
		"context":    cmd.contextDir,
		"dockerfile": filepath.Dir(file),
	}
}
