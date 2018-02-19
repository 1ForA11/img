package main

import (
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/jessfraz/img/runc"
	controlapi "github.com/moby/buildkit/api/services/control"
	"github.com/moby/buildkit/control"
	"github.com/moby/buildkit/frontend"
	"github.com/moby/buildkit/frontend/dockerfile"
	"github.com/moby/buildkit/identity"
	"github.com/moby/buildkit/session"
	"github.com/moby/buildkit/util/appcontext"
	"github.com/moby/buildkit/worker"
)

const buildShortHelp = `Build an image from a Dockerfile.`
const buildLongHelp = `
`

func (cmd *buildCommand) Name() string      { return "build" }
func (cmd *buildCommand) Args() string      { return "[OPTIONS] PATH" }
func (cmd *buildCommand) ShortHelp() string { return buildShortHelp }
func (cmd *buildCommand) LongHelp() string  { return buildLongHelp }
func (cmd *buildCommand) Hidden() bool      { return false }

func (cmd *buildCommand) Register(fs *flag.FlagSet) {
	fs.StringVar(&cmd.dockerfilePath, "f", "", "Name of the Dockerfile (Default is 'PATH/Dockerfile')")
	fs.StringVar(&cmd.tag, "t", "", "Name and optionally a tag in the 'name:tag' format")
	fs.StringVar(&cmd.target, "target", "", "Set the target build stage to build")
}

type buildCommand struct {
	contextDir     string
	dockerfilePath string
	target         string
	tag            string
}

func (cmd *buildCommand) Run(args []string) (err error) {
	if len(args) < 1 {
		return fmt.Errorf("must pass a path to build")
	}

	// Get the specified context.
	cmd.contextDir = args[0]

	// Parse what is set to come from stdin.
	if cmd.dockerfilePath == "-" {
		return errors.New("stdin not supported for Dockerfile yet")
	}

	if cmd.contextDir == "" {
		return errors.New("please specify build context (e.g. \".\" for the current directory)")
	}

	if cmd.contextDir == "-" {
		return errors.New("stdin not supported for build context yet")
	}

	// Create the controller.
	c, err := createBuildkitController(cmd.contextDir, cmd.dockerfilePath)
	if err != nil {
		return err
	}
	fmt.Printf("controller: %#v\n", c)

	// Create a temporary directory for the tar output.
	tmpTar, err := ioutil.TempFile("", "buldkit-build-using-dockerfile")
	if err != nil {
		return err
	}
	fmt.Printf("tmpTar: %s\n", tmpTar.Name())
	tmpTar.Close()
	//	defer os.Remove(tmpTar.Name())

	// Create the context.
	ctx := appcontext.Context()
	ref := identity.NewID()

	// Solve the dockerfile.
	resp, err := c.Solve(ctx, &controlapi.SolveRequest{
		Ref: ref,
		/*Exporter: "docker",
		ExporterAttrs: map[string]string{
			"name":   cmd.tag,
			"output": tmpTar.Name(),
		},*/
		Session:  ref,
		Frontend: "dockerfile.v0",
		FrontendAttrs: map[string]string{
			"filename": cmd.dockerfilePath,
			"target":   cmd.target,
		},
	})
	if err != nil {
		return fmt.Errorf("solving failed: %v", err)
	}
	fmt.Printf("solve response: %#v\n", resp)

	return nil
}

func createBuildkitController(contextDir, dockerfilePath string) (*control.Controller, error) {
	// Create the runc worker.
	opt, err := runc.NewWorkerOpt(defaultStateDirectory)
	if err != nil {
		return nil, fmt.Errorf("creating runc worker opt failed: %v", err)
	}

	// Set the session manager.
	sessionManager, err := session.NewManager()
	if err != nil {
		return nil, fmt.Errorf("creating session manager failed: %v", err)
	}
	opt.SessionManager = sessionManager

	w, err := runc.NewWorker(opt, contextDir, dockerfilePath)
	if err != nil {
		return nil, err
	}

	// Create the worker controller.
	wc := &worker.Controller{}
	if err = wc.Add(w); err != nil {
		return nil, err
	}

	// Add the frontends.
	frontends := map[string]frontend.Frontend{}
	frontends["dockerfile.v0"] = dockerfile.NewDockerfileFrontend()

	// Create the controller.
	return control.NewController(control.Opt{
		SessionManager:   sessionManager,
		WorkerController: wc,
		Frontends:        frontends,
		CacheExporter:    w.CacheExporter,
		CacheImporter:    w.CacheImporter,
	})
}

func defaultSessionName() string {
	wd, err := os.Getwd()
	if err != nil {
		return "unknown"
	}
	return filepath.Base(wd)
}
