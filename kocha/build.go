package main

import (
	"flag"
	"fmt"
	"go/build"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"text/template"
	"time"

	"github.com/naoina/kocha"
	"github.com/naoina/kocha/util"
)

// buildCommand implements `command` interface for `build` command.
type buildCommand struct {
	flag *flag.FlagSet

	// Whether the build as the True All-in-One binary.
	all bool

	// Version tag
	versionTag string
}

// Name returns name of `build` command.
func (c *buildCommand) Name() string {
	return "build"
}

// Alias returns alias of `build` command.
func (c *buildCommand) Alias() string {
	return "b"
}

// Short returns short description for help.
func (c *buildCommand) Short() string {
	return "build your application"
}

// Usage returns usage of `build` command.
func (c *buildCommand) Usage() string {
	return fmt.Sprintf(`%s [-a] [-tag TAG]`, c.Name())
}

func (c *buildCommand) DefineFlags(fs *flag.FlagSet) {
	fs.BoolVar(&c.all, "a", false, "make the true all-in-one binary")
	fs.StringVar(&c.versionTag, "tag", "", "specify version tag")
	c.flag = fs
}

// Run execute the process for `build` command.
func (c *buildCommand) Run() {
	dir, err := os.Getwd()
	if err != nil {
		panic(err)
	}
	appDir, err := util.FindAppDir()
	if err != nil {
		panic(err)
	}
	appName := filepath.Base(dir)
	configPkg, err := c.Package(path.Join(appDir, "config"))
	if err != nil {
		util.PanicOnError(c, "abort: cannot import `%s`: %v", path.Join(appDir, "config"), err)
	}
	var dbImportPath string
	dbPkg, err := c.Package(path.Join(appDir, "db"))
	if err == nil {
		dbImportPath = dbPkg.ImportPath
	}
	var migrationsImportPath string
	migrationsPkg, err := c.Package(path.Join(appDir, "db", "migrations"))
	if err == nil {
		migrationsImportPath = migrationsPkg.ImportPath
	}
	tmpDir, err := filepath.Abs("tmp")
	if err != nil {
		panic(err)
	}
	if err := os.Mkdir(tmpDir, 0755); err != nil && !os.IsExist(err) {
		util.PanicOnError(c, "abort: failed to create directory: %v", err)
	}
	_, filename, _, _ := runtime.Caller(0)
	baseDir := filepath.Dir(filename)
	skeletonDir := filepath.Join(baseDir, "skeleton", "build")
	mainTemplate, err := ioutil.ReadFile(filepath.Join(skeletonDir, "main.go.template"))
	if err != nil {
		panic(err)
	}
	mainFilePath := filepath.ToSlash(filepath.Join(tmpDir, "main.go"))
	builderFilePath := filepath.ToSlash(filepath.Join(tmpDir, "builder.go"))
	file, err := os.Create(builderFilePath)
	if err != nil {
		util.PanicOnError(c, "abort: failed to create file: %v", err)
	}
	defer file.Close()
	builderTemplatePath := filepath.ToSlash(filepath.Join(skeletonDir, "builder.go.template"))
	t := template.Must(template.ParseFiles(builderTemplatePath))
	var resources map[string]string
	if c.all {
		resources = c.collectResourcePaths(filepath.Join(dir, kocha.StaticDir))
	}
	data := map[string]interface{}{
		"configImportPath":     configPkg.ImportPath,
		"dbImportPath":         dbImportPath,
		"migrationsImportPath": migrationsImportPath,
		"mainTemplate":         string(mainTemplate),
		"mainFilePath":         mainFilePath,
		"resources":            resources,
		"version":              c.detectVersionTag(),
	}
	if err := t.Execute(file, data); err != nil {
		util.PanicOnError(c, "abort: failed to write file: %v", err)
	}
	execName := appName
	if runtime.GOOS == "windows" {
		execName += ".exe"
	}
	c.execCmd("go", "run", builderFilePath)
	c.execCmd("go", "build", "-o", execName, mainFilePath)
	if err := os.RemoveAll(tmpDir); err != nil {
		panic(err)
	}
	printSettingEnv()
	fmt.Printf("build all-in-one binary to %v\n", filepath.Join(dir, execName))
	util.PrintGreen("Build successful!\n")
}

func (c *buildCommand) Package(importPath string) (*build.Package, error) {
	pkg, err := build.Import(importPath, "", build.FindOnly)
	if err != nil {
		return nil, err
	}
	return pkg, nil
}

func (c *buildCommand) execCmd(cmd string, args ...string) {
	command := exec.Command(cmd, args...)
	if msg, err := command.CombinedOutput(); err != nil {
		util.PanicOnError(c, "abort: build failed: %v\n%v", err, string(msg))
	}
}

func (c *buildCommand) collectResourcePaths(root string) map[string]string {
	result := make(map[string]string)
	filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.Name()[0] == '.' {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if info.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		result[rel] = filepath.ToSlash(path)
		return nil
	})
	return result
}

func (c *buildCommand) detectVersionTag() string {
	if c.versionTag != "" {
		return c.versionTag
	}
	var repo string
	for _, dir := range []string{".git", ".hg"} {
		if info, err := os.Stat(dir); err == nil && info.IsDir() {
			repo = dir
			break
		}
	}
	version := time.Now().Format(time.RFC1123Z)
	switch repo {
	case ".git":
		bin, err := exec.LookPath("git")
		if err != nil {
			fmt.Println("WARNING: git repository found, but `git` command not found. version uses \"%s\"", version)
			break
		}
		line, err := exec.Command(bin, "rev-parse", "HEAD").Output()
		if err != nil {
			util.PanicOnError(c, "abort: unexpected error: %v\nplease specify version explicitly with '-tag' option for avoid the this error.", err)
		}
		version = strings.TrimSpace(string(line))
	case ".hg":
		bin, err := exec.LookPath("hg")
		if err != nil {
			fmt.Println("WARNING: hg repository found, but `hg` command not found. version uses \"%s\"", version)
			break
		}
		line, err := exec.Command(bin, "identify").Output()
		if err != nil {
			util.PanicOnError(c, "abort: unexpected error: %v\nplease specify version explicitly with '-tag' option for avoid the this error.", err)
		}
		version = strings.TrimSpace(string(line))
	}
	if version == "" {
		// Probably doesn't reach here.
		version = time.Now().Format(time.RFC1123Z)
		fmt.Println("WARNING: version is empty, use \"%s\"", version)
	}
	return version
}
