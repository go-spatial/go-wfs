///////////////////////////////////////////////////////////////////////////////
//
// The MIT License (MIT)
// Copyright (c) 2018 Jivan Amara
// Copyright (c) 2018 Tom Kralidis
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to
// deal in the Software without restriction, including without limitation the
// rights to use, copy, modify, merge, publish, distribute, sublicense, and/or
// sell copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND,
// EXPRESS OR IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF
// MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT.
// IN NO EVENT SHALL THE AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM,
// DAMAGES OR OTHER LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR
// OTHERWISE, ARISING FROM, OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE
// USE OR OTHER DEALINGS IN THE SOFTWARE.
//
///////////////////////////////////////////////////////////////////////////////

// go-wfs project main.go
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path"
	"path/filepath"
	"sort"

	"github.com/go-spatial/go-wfs/data_provider"
	"github.com/go-spatial/go-wfs/server"
	"github.com/go-spatial/tegola/provider/gpkg"
)

// For use when a data source isn't provided.
// Scans for files w/ '.gpkg' extension and chooses the first alphabetically.
// * First scan the working directory
// * Second, if none found in the working directory and it exists, scan ./data/
// * Third, if none found in either of the previous and it exists, scan ./test_data/
// Returns a full path to the file or an empty string if none found.
func defaultGpkg() string {
	// Directories to check in decreasing order of priority
	dirs := []string{"", "data", "test_data"}
	gpkgPath := ""
	wd, err := os.Getwd()
	if err != nil {
		log.Printf("Unable to get working directory: %v", err)
		return ""
	}

	for _, dir := range dirs {
		searchGlob := path.Join(dir, "*.gpkg")
		gpkgFiles, err := filepath.Glob(searchGlob)
		if err != nil {
			panic("Invalid glob pattern hardcoded")
		}
		if len(gpkgFiles) == 0 {
			continue
		}
		sort.Strings(gpkgFiles)
		gpkgFilename := gpkgFiles[0]
		gpkgPath = path.Clean(path.Join(wd, gpkgFilename))
		break
	}

	return gpkgPath
}

func main() {
	var bindIp string
	var bindPort int
	var serveAddress string
	var dataSource string

	flag.StringVar(&bindIp, "b", "127.0.0.1", "IP address for the server to listen on")
	flag.IntVar(&bindPort, "p", 9000, "port for the server to listen on")
	flag.StringVar(&serveAddress, "s", "", "IP:Port that connections will see the server at (defaults to bind address)")
	flag.StringVar(&dataSource , "d", "", "data source (path to .gpkg file)")

	flag.Parse()

	bindAddress := fmt.Sprintf("%v:%v", bindIp, bindPort)
	if serveAddress == "" {
		serveAddress = bindAddress
	}

	if dataSource != "" {
		if _, err := os.Stat(dataSource); os.IsNotExist(err) {
			panic("datasource does not exist")
		}
	}
	if dataSource == "" {
		dataSource = defaultGpkg()
	}
	if dataSource == "" {
		panic("no datasource")
	}
	dataConfig, err := gpkg.AutoConfig(dataSource)
	if err != nil {
		panic(fmt.Sprintf("data auto-config failure for '%v': %v", dataSource, err))
	}
	dataProvider, err := gpkg.NewTileProvider(dataConfig)
	if err != nil {
		panic(fmt.Sprintf("data provider creation error for '%v': %v", dataSource, err))
	}

	p := data_provider.Provider{Tiler: dataProvider}

	server.StartServer(bindAddress, serveAddress, p)
}
