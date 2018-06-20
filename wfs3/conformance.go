///////////////////////////////////////////////////////////////////////////////
//
// The MIT License (MIT)
// Copyright (c) 2018 Jivan Amara
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

// jivan project conformance.go

package wfs3

import (
	"encoding/json"
	"fmt"
	"hash/fnv"
	"log"
)

// --- Implements req/core/conformance-op
func Conformance() (content *ConformanceClasses, contentId string) {
	hasher := fnv.New64()
	content = &ConformanceClasses{
		ConformsTo: []string{
			"http://www.opengis.net/spec/wfs-1/3.0/req/core",
			"http://www.opengis.net/spec/wfs-1/3.0/req/geojson",
			// TODO: "http://www.opengis.net/spec/wfs-1/3.0/req/html",
		},
	}

	byteContent, err := json.Marshal(content)
	if err != nil {
		log.Printf("Problem marshaling content inside Conformance(): %v", err)
		return nil, ""
	}

	hasher.Write(byteContent)
	contentId = fmt.Sprintf("%x", hasher.Sum64())

	return content, contentId
}
