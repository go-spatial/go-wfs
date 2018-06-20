package wfs3

import (
	"fmt"
	"hash/fnv"

	"github.com/go-spatial/geom"
	"github.com/go-spatial/geom/encoding/geojson"
	"github.com/go-spatial/jivan/data_provider"
)

func FeatureData(cname string, fid uint64, p *data_provider.Provider, checkOnly bool) (content *Feature, contentId string, err error) {
	// TODO: This calculation of contentId assumes an unchanging data set.
	// 	When a changing data set is needed this will have to be updated, hopefully after data providers can tell us
	// 	something about updates.
	hasher := fnv.New64()
	hasher.Write([]byte(fmt.Sprintf("%v%v", cname, fid)))
	contentId = fmt.Sprintf("%x", hasher.Sum64())

	if checkOnly {
		return nil, contentId, nil
	}

	pfs, err := p.GetFeatures(
		[]data_provider.FeatureId{
			{Collection: cname, FeaturePk: fid},
		})
	if err != nil {
		return nil, "", err
	}

	if len(pfs) != 1 {
		return nil, "", fmt.Errorf("Invalid collection/fid: %v/%v", cname, fid)
	}

	pf := pfs[0]
	content = &Feature{
		Feature: geojson.Feature{
			ID: &pf.ID, Geometry: geojson.Geometry{Geometry: pf.Geometry}, Properties: pf.Properties,
		},
	}

	return content, contentId, nil
}

//
func FeatureCollectionData(cName string, bbox *geom.Extent, startIdx, stopIdx uint, properties map[string]string, p *data_provider.Provider, checkOnly bool) (content *FeatureCollection, featureTotal uint, contentId string, err error) {
	// TODO: This calculation of contentId assumes an unchanging data set.
	// 	When a changing data set is needed this will have to be updated, hopefully after data providers can tell us
	// 	something about updates.
	hasher := fnv.New64()
	hasher.Write([]byte(cName))
	contentId = fmt.Sprintf("%x", hasher.Sum64())

	if checkOnly {
		return nil, featureTotal, contentId, nil
	}

	// collection features filtered for matches in properties if it is non-nil, otherwise all
	cfs, err := p.CollectionFeatures(cName, properties, bbox)
	if err != nil {
		return nil, featureTotal, "", err
	}

	featureTotal = uint(len(cfs))
	originalStopIdx := stopIdx
	if stopIdx > featureTotal {
		stopIdx = featureTotal
	}

	if startIdx >= featureTotal || stopIdx < startIdx {
		return nil, featureTotal, "", fmt.Errorf(
			"Invalid start/stop indices [%v, %v] for collection of length %v", startIdx, originalStopIdx, featureTotal)
	}

	// Convert the provider features to geojson features.
	gfs := make([]geojson.Feature, stopIdx-startIdx)
	for i, pf := range cfs[startIdx:stopIdx] {
		gfs[i] = geojson.Feature{
			ID: &pf.ID, Geometry: geojson.Geometry{Geometry: pf.Geometry}, Properties: pf.Properties,
		}
	}

	// Wrap the features up in a FeatureCollection
	content = &FeatureCollection{
		FeatureCollection: geojson.FeatureCollection{Features: gfs},
	}

	return content, featureTotal, contentId, nil
}
