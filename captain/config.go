package captain // import "github.com/harbur/captain/captain"

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/v2/yaml"
)

// Config represents the information stored at captain.yml. It keeps information about images and unit tests.
type Config interface {
	GetImageNames() map[string]string
	GetUnitTestCommands() []string
	GetPreCommands() []string
	GetPostCommands() []string
}

type configV1 struct {
       Build  build
       Test   map[string][]string
       Images []string
       Root []string
}

type build struct {
       Images map[string]string
}

type config map[string]project

type project struct {
	Build  string
	Image  string
	Pre    []string
	Post   []string
	Test   []string
}

// configFile returns the file to read the config from.
// If the --config option was given,
// it will only use the given file.
func configFile(options Options) string {
	if len(options.config) > 0 {
		return options.config
	}
	return "captain.yml"
}

// readConfig will read the config file
// and return the created config.
func readConfig(filename string) *config {
	data, err := ioutil.ReadFile(filename)
	if err != nil {
		panic(StatusError{err, 74})
	}
	return unmarshal(data)
}

// displaySyntaxError will display more information
// such as line and error type given an error and
// the data that was unmarshalled.
// Thanks to https://github.com/markpeek/packer/commit/5bf33a0e91b2318a40c42e9bf855dcc8dd4cdec5
func displaySyntaxError(data []byte, syntaxError error) (err error) {
	syntax, ok := syntaxError.(*json.SyntaxError)
	if !ok {
		err = syntaxError
		return
	}
	newline := []byte{'\x0a'}
	space := []byte{' '}

	start, end := bytes.LastIndex(data[:syntax.Offset], newline)+1, len(data)
	if idx := bytes.Index(data[start:], newline); idx >= 0 {
		end = start + idx
	}

	line, pos := bytes.Count(data[:start], newline)+1, int(syntax.Offset)-start-1

	err = fmt.Errorf("\nError in line %d: %s \n%s\n%s^", line, syntaxError, data[start:end], bytes.Repeat(space, pos))
	return
}

// unmarshal converts either JSON
// or YAML into a config object.
func unmarshal(data []byte) *config {
	var configV1 *configV1
	res := yaml.Unmarshal(data, &configV1)
	if (len(configV1.Build.Images)>0) {
		err("Old %s format detected! Please check the https://github.com/harbur/captain how to upgrade", "captain.yml")
		os.Exit(-1)
	}

	var config *config
	res = yaml.Unmarshal(data, &config)

	if res != nil {
		res = displaySyntaxError(data, res)
		err("%s", res)
		os.Exit(InvalidCaptainYML)
	}

	return config
}

// NewConfig retus a new config based on given
// options.
// Containers will be ordered so that they can be
// brought up and down with Docker.
func NewConfig(options Options, forceOrder bool) Config {
	var conf *config
	f := configFile(options)
	if _, err := os.Stat(f); err == nil {
		conf = readConfig(f)
	}

	if conf == nil {
		info("No configuration found %v - inferring values", configFile(options))
		autoconf := make(config)
		conf = &autoconf
		dockerfiles := getDockerfiles()
		for build,image := range dockerfiles {
			autoconf[image] = project{Build:build, Image: image }
		}
	}

	var err error
	if err != nil {
		panic(StatusError{err, 78})
	}
	return conf
}

func (c *config) GetImageNames() map[string]string {
	// Get Image Builds
	var builds  []string
	for _,k := range *c {
		builds = append(builds, k.Build)
	}

	// Get Images
	var images = make(map[string]string)
	for _,k := range *c {
		images[k.Build] = k.Image
	}

	return images
}

func (c *config) GetUnitTestCommands() []string {
	var tests  []string
	for _,k := range *c {
		for _,t := range k.Test {
			tests = append(tests, t)
		}
	}
	return tests
}

func (c *config) GetPreCommands() []string {
	var pre  []string
	for _,k := range *c {
		for _,t := range k.Pre {
			pre = append(pre, t)
		}
	}
	return pre
}

func (c *config) GetPostCommands() []string {
	var post  []string
	for _,k := range *c {
		for _,t := range k.Post {
			post = append(post, t)
		}
	}
	return post
}

// Global list, how can I pass it to the visitor pattern?
var imagesMap = make(map[string]string)

func getDockerfiles() map[string]string {
	filepath.Walk(".", visit)
	return imagesMap
}

func visit(path string, f os.FileInfo, err error) error {
	// Filename is "Dockerfile" or has "Dockerfile." prefix and is not a directory
	if (f.Name() == "Dockerfile" || strings.HasPrefix(f.Name(), "Dockerfile.")) && !f.IsDir() {
		// Get Parent Dirname
		absolutePath, _ := filepath.Abs(path)
		var image = strings.ToLower(filepath.Base(filepath.Dir(absolutePath)))
		imagesMap[path] = options.namespace + "/" + image + strings.ToLower(filepath.Ext(path))
		debug("Located %s will be used to create %s", path, imagesMap[path])
	}
	return nil
}
