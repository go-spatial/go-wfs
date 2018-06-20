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

package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/go-spatial/geom"
	"github.com/go-spatial/jivan/config"
	"github.com/go-spatial/jivan/data_provider"
	"github.com/go-spatial/jivan/wfs3"
	"github.com/julienschmidt/httprouter"
)

// This is the default max number of features to return for feature collection reqeusts
const DEFAULT_RESULT_LIMIT = 10

const (
	HTTPStatusOk          = 200
	HTTPStatusNotModified = 304
	HTTPStatusServerError = 500
	HTTPStatusClientError = 400

	HTTPMethodGET  = "GET"
	HTTPMethodHEAD = "HEAD"
)

type HandlerError struct {
	Code        string `json:"code"`
	Description string `json:"description"`
}

// contentType() returns the Content-Type string that will be used for the response to this request.
// This Content-Type will be chosen in order of increasing priority from:
// request Content-Type, request Accept
// If the type chosen from the request isn't supported, defaultContentType will be used.
func supportedContentType(ct string) bool {
	supportedContentTypes := []string{config.JSONContentType, config.HTMLContentType}
	typeSupported := false
	for _, sct := range supportedContentTypes {
		if ct == sct {
			typeSupported = true
			break
		}
	}
	return typeSupported
}

func contentType(r *http.Request) string {
	defaultContentType := config.JSONContentType
	useType := ""
	ctType := r.Header.Get("Content-Type")
	acceptTypes := r.Header.Get("Accept")

	if supportedContentType(ctType) {
		useType = ctType
	}

	// TODO: Parse acceptTypes properly
	acceptTypes = acceptTypes

	// if query string 'f' parameter is passed
	// override HTTP Accept header
	q := r.URL.Query()
	qFormat := q["f"]

	if len(qFormat) > 0 {
		if qFormat[0] != useType {
			useType = qFormat[0]
		}
	}

	if !supportedContentType(useType) {
		useType = defaultContentType
	}

	return useType
}

// Sets response 'status', and writes a json-encoded object with property "description" having value "msg".
func jsonError(w http.ResponseWriter, code string, msg string, status int) {
	w.WriteHeader(status)

	result, err := json.Marshal(struct {
		Code        string `json:"code"`
		Description string `json:"description"`
	}{
		Code:        code,
		Description: msg,
	})

	if err != nil {
		w.Write([]byte(fmt.Sprintf("problem marshaling error: %v", msg)))
	} else {
		w.Write(result)
	}
}

// Provides a link for the given content type
func ctLink(baselink, contentType string) string {
	if !supportedContentType(contentType) {
		panic(fmt.Sprintf("unsupported content type: %v", contentType))
	}

	u, err := url.Parse(baselink)
	if err != nil {
		log.Printf("Invalid link '%v', will return empty string.", baselink)
		return ""
	}
	q := u.Query()

	var l string
	switch contentType {
	case config.Configuration.Server.DefaultMimeType:
	default:
		q["f"] = []string{contentType}
	}

	u.RawQuery = q.Encode()
	l = u.String()
	return l
}

// Serves the root content for WFS3.
func root(w http.ResponseWriter, r *http.Request) {
	ct := contentType(r)
	rPath := "/"
	// This allows tests to set the result to whatever they want.
	overrideContent := r.Context().Value("overrideContent")

	rootContent, contentId := wfs3.Root(false)

	sshpb := serveSchemeHostPortBase(r)
	apiUrl := fmt.Sprintf("%v/api", sshpb)
	conformanceUrl := fmt.Sprintf("%v/conformance", sshpb)
	collectionsUrl := fmt.Sprintf("%v/collections", sshpb)
	rootUrl := fmt.Sprintf("%v/", sshpb)

	alttypes := []string{}
	switch ct {
	case config.JSONContentType:
		alttypes = append(alttypes, config.HTMLContentType)
	case config.HTMLContentType:
		alttypes = append(alttypes, config.JSONContentType)
	}

	var links []*wfs3.Link
	links = append(links, &wfs3.Link{Href: ctLink(rootUrl, ct), Rel: "self", Type: ct})
	for _, at := range alttypes {
		links = append(links, &wfs3.Link{Href: ctLink(rootUrl, at), Rel: "alternate", Type: at})
	}
	links = append(links, &wfs3.Link{Href: ctLink(apiUrl, ct), Rel: "service", Type: ct})
	links = append(links, &wfs3.Link{Href: ctLink(conformanceUrl, ct), Rel: "conformance", Type: ct})
	links = append(links, &wfs3.Link{Href: ctLink(collectionsUrl, ct), Rel: "data", Type: ct})

	rootContent.Links = links

	w.Header().Set("ETag", contentId)
	if r.Method == HTTPMethodHEAD {
		if r.Header.Get("ETag") == contentId {
			w.WriteHeader(HTTPStatusNotModified)
		} else {
			w.WriteHeader(HTTPStatusOk)
		}
		return
	}

	var encodedContent []byte
	var err error
	if ct == config.JSONContentType {
		encodedContent, err = json.Marshal(rootContent)
	} else if ct == config.HTMLContentType {
		encodedContent, err = rootContent.MarshalHTML(config.Configuration)
	} else {
		jsonError(w, "InvalidParameterValue", "Content-Type: '"+ct+"' not supported.", HTTPStatusServerError)
		return
	}

	if err != nil {
		jsonError(w, "NoApplicableCode", err.Error(), HTTPStatusServerError)
		return
	}

	w.Header().Set("Content-Type", ct)

	if overrideContent != nil {
		encodedContent = overrideContent.([]byte)
	}

	if ct == config.JSONContentType {
		respBodyRC := ioutil.NopCloser(bytes.NewReader(encodedContent))
		err = wfs3.ValidateJSONResponse(r, rPath, HTTPStatusOk, w.Header(), respBodyRC)
		if err != nil {
			log.Printf("%v", err)
			jsonError(w, "NoApplicableCode", "response doesn't match schema", HTTPStatusServerError)
			return
		}
	}

	w.WriteHeader(HTTPStatusOk)
	w.Write(encodedContent)
}

func conformance(w http.ResponseWriter, r *http.Request) {
	cPath := "/conformance"
	// This allows tests to set the result to whatever they want.
	overrideContent := r.Context().Value("overrideContent")

	ct := contentType(r)
	c, contentId := wfs3.Conformance()
	w.Header().Set("ETag", contentId)
	if r.Method == HTTPMethodHEAD {
		if r.Header.Get("ETag") == contentId {
			w.WriteHeader(HTTPStatusNotModified)
		} else {
			w.WriteHeader(HTTPStatusOk)
		}
		return
	}

	var encodedContent []byte
	var err error
	if ct == config.JSONContentType {
		encodedContent, err = json.Marshal(c)
	} else if ct == config.HTMLContentType {
		encodedContent, err = c.MarshalHTML(config.Configuration)
	} else {
		jsonError(w, "InvalidParameterValue", "Content-Type: ''"+ct+"'' not supported.", HTTPStatusServerError)
		return
	}

	if err != nil {
		msg := fmt.Sprintf("problem marshaling conformance declaration to %v: %v", ct, err.Error())
		jsonError(w, "NoApplicableCode", msg, HTTPStatusServerError)
		return
	}

	w.Header().Set("Content-Type", ct)

	if overrideContent != nil {
		encodedContent = overrideContent.([]byte)
	}
	if ct == config.JSONContentType {
		respBodyRC := ioutil.NopCloser(bytes.NewReader(encodedContent))
		err = wfs3.ValidateJSONResponse(r, cPath, HTTPStatusOk, w.Header(), respBodyRC)
		if err != nil {
			log.Printf(fmt.Sprintf("%v", err))
			jsonError(w, "NoApplicableCode", "response doesn't match schema", HTTPStatusServerError)
			return
		}
	}

	w.WriteHeader(HTTPStatusOk)
	w.Write(encodedContent)
}

// --- Return the json-encoded OpenAPI 3 spec for the WFS API available on this instance.
func openapi(w http.ResponseWriter, r *http.Request) {
	// --- TODO: Disabled due to #34
	// oapiPath := "/api"
	// This allows tests to set the result to whatever they want.
	overrideContent := r.Context().Value("overrideContent")

	ct := contentType(r)

	if ct != config.JSONContentType {
		jsonError(w, "InvalidParameterValue", "Content-Type: ''"+ct+"'' not supported.", HTTPStatusServerError)
		return
	}
	encodedContent, contentId := wfs3.OpenAPI3SchemaEncoded(ct)
	w.Header().Set("ETag", contentId)

	if r.Method == HTTPMethodHEAD {
		if r.Header.Get("ETag") == contentId {
			w.WriteHeader(HTTPStatusNotModified)
		} else {
			w.WriteHeader(HTTPStatusOk)
		}
		return
	}

	w.Header().Set("Content-Type", ct)

	if overrideContent != nil {
		encodedContent = overrideContent.([]byte)
	}

	// TODO: As of 2018-04-05 I can't find a reliable openapi3 document schema.  When one is published use if for validation here.
	// if ct == config.JSONContentType {
	// 	err := wfs3.ValidateJSONResponseAgainstJSONSchema(encodedContent, jsonSchema)
	// 	if err != nil {
	// 		log.Printf(fmt.Sprintf("%v", err))
	// 		jsonError(w, "NoApplicableCode", "response doesn't match schema", HTTPStatusServerError)
	// 		return
	// 	}
	// } else {
	// 	msg := fmt.Sprintf("unsupported content type: %v", ct)
	// 	log.Printf(msg)
	// 	jsonError(w, "InvalidParametrValue", msg, HTTPStatusClientError)
	// }

	w.WriteHeader(HTTPStatusOk)
	w.Write(encodedContent)
}

func collectionMetaData(w http.ResponseWriter, r *http.Request) {
	cmdPath := "/collections/{name}"
	overrideContent := r.Context().Value("overrideContent")

	ct := contentType(r)
	ps := httprouter.ParamsFromContext(r.Context())

	cName := ps.ByName("name")
	if cName == "" {
		jsonError(w, "MissingParameterValue", "No {name} provided", HTTPStatusClientError)
		return
	}

	md, contentId, err := wfs3.CollectionMetaData(cName, &Provider, serveSchemeHostPortBase(r), false)
	if err != nil {
		jsonError(w, "NoApplicableCode", err.Error(), HTTPStatusServerError)
		return
	}

	collectionMdUrlBase := fmt.Sprintf("%v/collections/%v", serveSchemeHostPortBase(r), cName)
	collectionDataUrlBase := fmt.Sprintf("%v/collections/%v/items", serveSchemeHostPortBase(r), cName)
	altcts := []string{}
	switch ct {
	case config.JSONContentType:
		altcts = append(altcts, config.HTMLContentType)
	case config.HTMLContentType:
		altcts = append(altcts, config.JSONContentType)
	default:
		jsonError(w, "InvalidParamaterValue", "Content-Type: ''"+ct+"'' not supported.", HTTPStatusServerError)
	}
	// Prepend these self-pointing links to md.Links
	plinks := []*wfs3.Link{}
	plinks = append(plinks, &wfs3.Link{Rel: "self", Href: ctLink(collectionMdUrlBase, ct), Type: ct})
	for _, act := range altcts {
		plinks = append(plinks, &wfs3.Link{Rel: "alternate", Href: ctLink(collectionMdUrlBase, act), Type: act})
	}
	// Include these links to actual data
	plinks = append(plinks, &wfs3.Link{Rel: "item", Href: ctLink(collectionDataUrlBase, ct), Type: ct})
	for _, act := range altcts {
		plinks = append(plinks, &wfs3.Link{Rel: "item", Href: ctLink(collectionDataUrlBase, act), Type: act})
	}
	md.Links = append(plinks, md.Links...)

	w.Header().Set("ETag", contentId)
	if r.Method == HTTPMethodHEAD {
		if r.Header.Get("ETag") == contentId {
			w.WriteHeader(HTTPStatusNotModified)
		} else {
			w.WriteHeader(HTTPStatusOk)
		}
		return
	}

	var encodedContent []byte
	if ct == config.JSONContentType {
		md.ContentType(ct)
		encodedContent, err = json.Marshal(md)
	} else if ct == config.HTMLContentType {
		encodedContent, err = md.MarshalHTML(config.Configuration)
	} else {
		jsonError(w, "InvalidParamaterValue", "Content-Type: ''"+ct+"'' not supported.", HTTPStatusServerError)
		return
	}

	if err != nil {
		jsonError(w, "NoApplicableCode", err.Error(), HTTPStatusServerError)
		return
	}

	w.Header().Set("Content-Type", ct)

	if overrideContent != nil {
		encodedContent = overrideContent.([]byte)
	}

	if ct == config.JSONContentType {
		respBodyRC := ioutil.NopCloser(bytes.NewReader(encodedContent))
		err = wfs3.ValidateJSONResponse(r, cmdPath, HTTPStatusOk, w.Header(), respBodyRC)
		if err != nil {
			log.Printf(fmt.Sprintf("%v", err))
			jsonError(w, "NoApplicableCode", "response doesn't match schema", HTTPStatusServerError)
			return
		}
	}

	w.WriteHeader(HTTPStatusOk)
	w.Write(encodedContent)
}

func collectionsMetaData(w http.ResponseWriter, r *http.Request) {
	cmdPath := "/collections"
	overrideContent := r.Context().Value("overrideContent")

	ct := contentType(r)
	md, contentId, err := wfs3.CollectionsMetaData(&Provider, serveSchemeHostPortBase(r), false)
	if err != nil {
		jsonError(w, "NoApplicableCode", err.Error(), HTTPStatusServerError)
		return
	}

	w.Header().Set("ETag", contentId)
	if r.Method == HTTPMethodHEAD {
		if r.Header.Get("ETag") == contentId {
			w.WriteHeader(HTTPStatusNotModified)
		} else {
			w.WriteHeader(HTTPStatusOk)
		}
		return
	}

	// This needs to be done before adding the alternate links below, otherwise they will all be
	//	converted to ct
	md.ContentType(ct)

	// Add self link to beginning of Links
	selfHrefBase := fmt.Sprintf("%v%v", serveSchemeHostPortBase(r), cmdPath)
	selfLink := &wfs3.Link{Rel: "self", Href: ctLink(selfHrefBase, ct), Type: ct}

	// Add alternative links after self link
	altLinks := make([]*wfs3.Link, 0, 5)
	for _, sct := range config.SupportedContentTypes {
		if ct == sct {
			continue
		}
		altLinks = append(altLinks, &wfs3.Link{Rel: "alternate", Href: ctLink(selfHrefBase, sct), Type: sct})
	}

	// Add item links after alt links
	ilinks := make([]*wfs3.Link, 0, len(md.Collections))
	for _, c := range md.Collections {
		chref := fmt.Sprintf("%v/%v", selfHrefBase, c.Name)
		// self & alternate links
		c.Links = append(c.Links, &wfs3.Link{Rel: "self", Href: ctLink(chref, ct), Type: ct})
		ilinks = append(ilinks, &wfs3.Link{Rel: "item", Href: ctLink(chref, ct), Type: ct})
		for _, sct := range config.SupportedContentTypes {
			if ct == sct {
				continue
			}
			c.Links = append(c.Links, &wfs3.Link{Rel: "alternate", Href: ctLink(chref, sct), Type: sct})
			ilinks = append(ilinks, &wfs3.Link{Rel: "item", Href: ctLink(chref, sct), Type: sct})
		}
		// item links
		ihref := fmt.Sprintf("%v/%v/items", selfHrefBase, c.Name)
		for _, sct := range config.SupportedContentTypes {
			c.Links = append(c.Links, &wfs3.Link{Rel: "item", Href: ctLink(ihref, sct), Type: sct})
		}
	}
	links := []*wfs3.Link{selfLink}
	links = append(links, altLinks...)
	links = append(links, md.Links...)
	links = append(links, ilinks...)
	md.Links = links

	var encodedContent []byte
	if ct == config.JSONContentType {
		encodedContent, err = json.Marshal(md)
	} else if ct == config.HTMLContentType {
		encodedContent, err = md.MarshalHTML(config.Configuration)
	} else {
		jsonError(w, "InvalidParameterValue", "Content-Type: ''"+ct+"'' not supported.", HTTPStatusServerError)
		return
	}

	if err != nil {
		jsonError(w, "NoApplicableCode", err.Error(), HTTPStatusServerError)
		return
	}

	w.Header().Set("Content-Type", ct)

	if overrideContent != nil {
		encodedContent = overrideContent.([]byte)
	}

	if ct == config.JSONContentType {
		respBodyRC := ioutil.NopCloser(bytes.NewReader(encodedContent))
		err = wfs3.ValidateJSONResponse(r, cmdPath, HTTPStatusOk, w.Header(), respBodyRC)
		if err != nil {
			log.Printf(fmt.Sprintf("%v", err))
			jsonError(w, "NoApplicableCode", "response doesn't match schema", HTTPStatusServerError)
			return
		}
	}

	w.WriteHeader(HTTPStatusOk)
	w.Write(encodedContent)
}

// --- Provide paged access to data for all features at /collections/{name}/items/{feature_id}
func collectionData(w http.ResponseWriter, r *http.Request) {
	ct := contentType(r)
	overrideContent := r.Context().Value("overrideContent")

	urlParams := httprouter.ParamsFromContext(r.Context())
	cName := urlParams.ByName("name")
	fidStr := urlParams.ByName("feature_id")
	var fid uint64
	var err error
	if fidStr != "" {
		cid, err := strconv.Atoi(fidStr)
		if err != nil {
			jsonError(w, "InvalidParameterValue", "Invalid feature_id: "+fidStr, HTTPStatusClientError)
		}
		fid = uint64(cid)
	}

	q := r.URL.Query()
	reservedQParams := []string{"f", "page", "limit", "time", "bbox"}
	var limit, pageNum uint
	var timeprops map[string]string

	qPageSize := q["limit"]
	if len(qPageSize) != 1 {
		limit = DEFAULT_RESULT_LIMIT
	} else {
		ps, err := strconv.ParseUint(qPageSize[0], 10, 64)
		if err != nil {
			jsonError(w, "NoApplicableCode", err.Error(), HTTPStatusClientError)
			return
		}
		if ps > uint64(config.Configuration.Server.MaxLimit) {
			ps = uint64(config.Configuration.Server.MaxLimit)
		}
		limit = uint(ps)
	}

	qPageNum := q["page"]
	if len(qPageNum) != 1 {
		pageNum = 0
	} else {
		pn, err := strconv.ParseUint(qPageNum[0], 10, 64)
		if err != nil {
			jsonError(w, "NoApplicableCode", err.Error(), HTTPStatusClientError)
			return
		}
		pageNum = uint(pn)
	}

	qBBox := q["bbox"]
	var bbox *geom.Extent
	if len(qBBox) > 0 {
		if len(qBBox) > 1 {
			jsonError(w, "InvalidParameterValue", "'bbox' parameter provided more than once", HTTPStatusClientError)
			return
		}

		bbox_items := strings.Split(qBBox[0], ",")
		if len(bbox_items) != 4 {
			msg := fmt.Sprintf("'bbox' parameter has %v items, expecting 4: '%v'", len(bbox_items), qBBox[0])
			jsonError(w, "InvalidParameterValue", msg, HTTPStatusClientError)
			return
		} else {
			bbox = &geom.Extent{}
			for i, p := range bbox_items {
				if bbox[i], err = strconv.ParseFloat(p, 64); err != nil {
					msg := fmt.Sprintf("'bbox' parameter has invalid format for item %v/4: '%v' / '%v'", i+1, p, qBBox[0])
					jsonError(w, "InvalidParameterValue", msg, HTTPStatusClientError)
					return
				}
			}
		}
	}

	qTime := q["time"]
	if len(qTime) > 0 {
		if len(qTime) > 1 {
			jsonError(w, "InvalidParameterValue", "'time' parameter provided more than once'", HTTPStatusClientError)
			return
		}
		ts := strings.Split(qTime[0], "/")
		timeprops = make(map[string]string)
		if len(ts) == 1 {
			timeprops["timestamp"] = ts[0]
		} else if len(ts) == 2 {
			timeprops["start_time"] = ts[0]
			timeprops["stop_time"] = ts[1]
		} else {
			jsonError(w, "InvalidParameterValue", "'time' parameter contains more than two time values ('/' separator)", HTTPStatusClientError)
			return
		}
	}

	// Collect additional property filters
	properties := make(map[string]string)
NEXT_QUERY_PARAM:
	for k, v := range q {
		for _, rqp := range reservedQParams {
			if k == rqp {
				continue NEXT_QUERY_PARAM
			}
		}

		properties[k] = v[0]
	}

	// Add time-specific properties
	for k, v := range timeprops {
		properties[k] = v
	}

	var data interface{}
	var jsonSchema string
	// Hex string hash of content
	var contentId string
	// Indicates if there is more data available from stopIdx onward
	var featureTotal uint
	// If a feature_id was provided, get a single feature, otherwise get a feature collection
	//	containing all of the collection's features
	if fidStr != "" {
		data, contentId, err = wfs3.FeatureData(cName, fid, &Provider, false)
		jsonSchema = wfs3.FeatureJSONSchema
	} else {
		// First index we're interested in
		startIdx := limit * pageNum
		// Last index we're interested in +1
		stopIdx := startIdx + limit

		data, featureTotal, contentId, err = wfs3.FeatureCollectionData(cName, bbox, startIdx, stopIdx, properties, &Provider, false)
		jsonSchema = wfs3.FeatureCollectionJSONSchema
	}

	if err != nil {
		var sc int
		var msg string
		switch e := err.(type) {
		case *data_provider.BadTimeString:
			msg = e.Error()
			sc = HTTPStatusClientError
		default:
			msg = fmt.Sprintf("Problem collecting feature data: %v", e)
			sc = HTTPStatusServerError
		}
		jsonError(w, "InvalidParameterValue", msg, sc)
		return
	}

	w.Header().Set("ETag", contentId)
	if r.Method == HTTPMethodHEAD {
		if r.Header.Get("ETag") == contentId {
			w.WriteHeader(HTTPStatusNotModified)
		} else {
			w.WriteHeader(HTTPStatusOk)
		}
		return
	}

	// Alternate content types
	var altcts []string
	switch ct {
	case config.JSONContentType:
		altcts = append(altcts, config.HTMLContentType)
	case config.HTMLContentType:
		altcts = append(altcts, config.JSONContentType)
	}

	var encodedContent []byte
	switch d := data.(type) {
	case *wfs3.Feature:
		// Generate links
		shref := fmt.Sprintf("%v/collections/%v/items/%v", serveSchemeHostPortBase(r), cName, fid)
		for _, sct := range config.SupportedContentTypes {
			rel := "alternate"
			if sct == ct {
				rel = "self"
			}
			d.Links = append(d.Links, &wfs3.Link{Rel: rel, Href: ctLink(shref, sct), Type: sct})
		}
		chref := fmt.Sprintf("%v/collections/%v", serveSchemeHostPortBase(r), cName)
		d.Links = append(d.Links, &wfs3.Link{Rel: "collection", Href: ctLink(chref, ct), Type: ct})

		if ct == config.JSONContentType {
			encodedContent, err = json.Marshal(d)
		} else if ct == config.HTMLContentType {
			encodedContent, err = d.MarshalHTML(config.Configuration)
		} else {
			jsonError(w, "InvalidParameterValue", "Content-Type: ''"+ct+"'' not supported.", HTTPStatusServerError)
			return
		}
	case *wfs3.FeatureCollection:
		// Generate self, previous, and next links
		self := fmt.Sprintf(
			"%v/collections/%v/items?page=%v&limit=%v", serveSchemeHostPortBase(r), cName, pageNum, limit)
		var prev string
		var next string
		if pageNum > 0 {
			prev = fmt.Sprintf(
				"%v/collections/%v/items?page=%v&limit=%v", serveSchemeHostPortBase(r), cName, pageNum-1, limit)
			purl, err := url.Parse(prev)
			if err != nil {
				jsonError(w, "NoApplicableCode", "problem parsing generated 'prev' link", 500)
				return
			}
			purl.RawQuery = purl.Query().Encode()
			prev = purl.String()
		}
		if featureTotal > (limit * (pageNum + 1)) {
			next = fmt.Sprintf(
				"%v/collections/%v/items?page=%v&limit=%v", serveSchemeHostPortBase(r), cName, pageNum+1, limit)
			nurl, err := url.Parse(next)
			if err != nil {
				jsonError(w, "NoApplicableCode", "problem parsing generated 'next' link", 500)
				return
			}
			nurl.RawQuery = nurl.Query().Encode()
			next = nurl.String()
		}

		d.Links = append(d.Links, &wfs3.Link{Rel: "self", Href: ctLink(self, ct), Type: ct})
		var alts = []*wfs3.Link{}
		for _, act := range altcts {
			alts = append(alts, &wfs3.Link{Rel: "alternate", Href: ctLink(self, act), Type: act})
		}
		d.Links = append(d.Links, alts...)
		if prev != "" {
			d.Links = append(d.Links, &wfs3.Link{Rel: "prev", Href: prev, Type: ct})
		}
		if next != "" {
			d.Links = append(d.Links, &wfs3.Link{Rel: "next", Href: next, Type: ct})
		}
		d.NumberMatched = featureTotal
		d.NumberReturned = uint(len(d.Features))

		if ct == config.JSONContentType {
			encodedContent, err = json.Marshal(d)
		} else if ct == config.HTMLContentType {
			encodedContent, err = d.MarshalHTML(config.Configuration)
		} else {
			jsonError(w, "InvalidParameterValue", "Content-Type: ''"+ct+"'' not supported.", HTTPStatusServerError)
			return
		}
	default:
		msg := fmt.Sprintf("Unexpected feature data type: %T, %v", data, data)
		jsonError(w, "NoApplicableCode", msg, HTTPStatusServerError)
		return
	}

	if err != nil {
		msg := fmt.Sprintf("Problem marshalling feature data: %v", err)
		jsonError(w, "oApplicableCode", msg, HTTPStatusServerError)
	}

	w.Header().Set("Content-Type", ct)

	if overrideContent != nil {
		encodedContent = overrideContent.([]byte)
	}

	if ct == config.JSONContentType {
		err = wfs3.ValidateJSONResponseAgainstJSONSchema(encodedContent, jsonSchema)
		if err != nil {
			log.Printf(fmt.Sprintf("%v", err))
			jsonError(w, "NoApplicableCode", "response doesn't match schema", HTTPStatusServerError)
			return
		}
	}

	w.WriteHeader(HTTPStatusOk)
	w.Write(encodedContent)
}

// --- Create temporary collection w/ filtered features.
// Returns a collection id for inspecting the resulting features.
func filteredFeatures(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	extentParam := q["extent"]
	collectionParam := q["collection"]

	// Grab any params besides "extent" & "collection" as property filters.
	propParams := make(map[string]string, len(q))
	for k, v := range r.URL.Query() {
		if k == "extent" || k == "collection" {
			continue
		}
		propParams[k] = v[0]
		if len(v) > 1 {
			log.Printf("Got multiple values for property filter, will only use the first '%v': %v", k, v)
		}
	}

	var collectionNames []string
	if len(collectionParam) > 0 {
		collectionNames = collectionParam
	} else {
		var err error
		collectionNames, err = Provider.CollectionNames()
		if err != nil {
			jsonError(w, "NoApplicableCode", err.Error(), HTTPStatusServerError)
		}
	}

	var extent geom.Extent
	if len(extentParam) > 0 {
		// lat/lon bounding box arranged as [<minx>, <miny>, <maxx>, <maxy>]
		var llbbox [4]float64
		err := json.Unmarshal([]byte(extentParam[0]), &llbbox)
		if err != nil {
			jsonError(w, "NoApplicableCode", fmt.Sprintf("unable to unmarshal extent (%v) due to error: %v", extentParam[0], err), HTTPStatusClientError)
			return
		}
		extent = geom.Extent{llbbox[0], llbbox[1], llbbox[2], llbbox[3]}
		// TODO: filter by extent
		if len(extentParam) > 1 {
			log.Printf("Multiple extent filters, will only use the first '%v'", extentParam)
		}
	}

	fids, err := Provider.FilterFeatures(&extent, collectionNames, propParams)
	newCol, err := Provider.MakeCollection("tempcol", fids)

	if err != nil {
		jsonError(w, "NoApplicableCode", err.Error(), HTTPStatusServerError)
		return
	}

	resp, err := json.Marshal(struct {
		Collection   string
		FeatureCount int
	}{Collection: newCol, FeatureCount: len(fids)})
	if err != nil {
		jsonError(w, "NoApplicableCode", err.Error(), HTTPStatusServerError)
	}
	w.WriteHeader(HTTPStatusOk)
	w.Write(resp)
}
