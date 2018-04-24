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

// go-wfs project collection_meta_data.go

package wfs3

import (
	"fmt"
	"hash/fnv"
	"log"

	"github.com/go-spatial/go-wfs/data_provider"
)

func CollectionsMetaData(p *data_provider.Provider, serveAddress string, checkOnly bool) (content *CollectionsInfo, contentId string, err error) {
	// TODO: This calculation of contentId assumes an unchanging data set.
	// 	When a changing data set is needed this will have to be updated, hopefully after data providers can tell us
	// 	something about updates.
	hasher := fnv.New64()
	hasher.Write([]byte(fmt.Sprintf("%v", serveAddress)))
	contentId = fmt.Sprintf("%x", hasher.Sum64())
	if checkOnly {
		return nil, contentId, nil
	}

	cNames, err := p.CollectionNames()
	if err != nil {
		// TODO: Log error
		return nil, "", err
	}

	csInfo := CollectionsInfo{Links: []*Link{}, Collections: []*CollectionInfo{}}
	for _, cn := range cNames {
		collectionUrl := fmt.Sprintf("%v/collections/%v", serveAddress, cn)
		cInfo := CollectionInfo{Name: cn, Links: []*Link{{Rel: "self", Href: collectionUrl}}}
		cLink := Link{Href: collectionUrl, Rel: "item"}

		csInfo.Links = append(csInfo.Links, &cLink)
		csInfo.Collections = append(csInfo.Collections, &cInfo)

		// add HTML representations
		collectionUrlHtml := fmt.Sprintf("%v/collections/%v?f=text/html", serveAddress, cn)
		//cInfoHtml := CollectionInfo{Name: cn, Links: []*Link{{Rel: "alternate", Href: collectionUrlHtml, Type: "text/html"}}}
		cLinkHtml := Link{Href: collectionUrlHtml, Rel: "item", Type: "text/html"}

		csInfo.Links = append(csInfo.Links, &cLinkHtml)
	}

	return &csInfo, contentId, nil
}

func CollectionMetaData(name string, p *data_provider.Provider, serveAddress string, checkOnly bool) (content *CollectionInfo, contentId string, err error) {
	// TODO: This calculation of contentId assumes an unchanging data set.
	// 	When a changing data set is needed this will have to be updated, hopefully after data providers can tell us
	// 	something about updates.
	hasher := fnv.New64()
	hasher.Write([]byte(fmt.Sprintf("%v%v", serveAddress, name)))
	contentId = fmt.Sprintf("%x", hasher.Sum64())
	if checkOnly {
		return nil, contentId, nil
	}

	cNames, err := p.CollectionNames()
	if err != nil {
		log.Printf("problem getting collection names: %v", err)
		return nil, "", err
	}

	validName := false
	for _, cName := range cNames {
		if name == cName {
			validName = true
			break
		}
	}
	if !validName {
		return nil, "", fmt.Errorf("Invalid collection name: %v", name)
	}

	collectionUrl := fmt.Sprintf("%v/collections/%v", serveAddress, name)
	cInfo := CollectionInfo{Name: name, Title: name, Links: []*Link{{Rel: "self", Href: collectionUrl}}}

	return &cInfo, contentId, nil
}
