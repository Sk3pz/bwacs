package main

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
)

type Aircraft struct {
	Hex          string      `json:"hex"`
	Callsign     string      `json:"flight"`
	Type         string      `json:"t"`
	Squawk       string      `json:"squawk"`
	Registration string      `json:"r"`
	Latitude     float64     `json:"lat"`
	Longitude    float64     `json:"lon"`
	Speed        float64     `json:"gs"`
	Altitude     interface{} `json:"alt_baro"`
}

func (a *Aircraft) getAlt() int {
	switch v := a.Altitude.(type) {
	case float64:
		return int(v)
	default:
		// on ground
		return -1
	}
}

func (a *Aircraft) display() string {
	rawAlt := a.getAlt()
	var alt string
	if rawAlt == -1 {
		alt = "ground"
	} else if rawAlt < 10000 {
		// don't need to put it in flight level
		alt = fmt.Sprintf("%dft", rawAlt)
	} else {
		alt = fmt.Sprintf("fl%d", feetToFlightLevel(rawAlt))
	}
	return fmt.Sprintf("%s \"%s\": %s registration %s\n @ lat/long %f/%f squawking %s\n %.1fkt @ %s",
		a.Hex, a.Callsign, a.Type, a.Registration, a.Latitude, a.Longitude, a.Squawk, a.Speed, alt)
}

func feetToFlightLevel(altitude int) int {
	// Flight level is the altitude in feet divided by 100, rounded to the nearest integer
	return altitude / 100
}

// todo: this does not handle Non-ICAO hex's, add feature to optionally ping for Non-ICAO as they *can* be military jets
func getMilitaryAircraft() []Aircraft {
	url := "https://adsbexchange-com1.p.rapidapi.com/v2/mil/"

	req, _ := http.NewRequest("GET", url, nil)

	req.Header.Add("X-RapidAPI-Key", "ae81000128mshac861a9f4874ef1p161e9ejsn8ab47b4ac100")
	req.Header.Add("X-RapidAPI-Host", "adsbexchange-com1.p.rapidapi.com")

	res, _ := http.DefaultClient.Do(req)
	if res == nil {
		return nil
	}

	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {
			_ = fmt.Errorf("failed to close HTTP body: %v", err)
		}
	}(res.Body)
	body, _ := io.ReadAll(res.Body)

	type Response struct {
		Aircraft []Aircraft `json:"ac"`
	}

	var data Response
	if err := json.Unmarshal(body, &data); err != nil {
		fmt.Println("\nJSON decode error:", err)
		return nil
	}
	return data.Aircraft
}

// get distance between two points on earth in NM
func haversine(lat1, long1, lat2, long2 float64) float64 {
	// Convert latitudes and longitudes to radians
	lat1 = toRadians(lat1)
	long1 = toRadians(long1)
	lat2 = toRadians(lat2)
	long2 = toRadians(long2)

	// Calculate the difference between the latitudes and longitudes
	dLat := lat2 - lat1
	dLong := long2 - long1

	// Calculate the distance using the Haversine formula
	a := math.Pow(math.Sin(dLat/2), 2) + math.Cos(lat1)*math.Cos(lat2)*math.Pow(math.Sin(dLong/2), 2)
	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
	const earthRadius = 3440.1 // nm
	return earthRadius * c
}

func toRadians(degrees float64) float64 {
	return degrees * math.Pi / 180
}

func filterAircraftInRadius(ac []Aircraft, lat, long, radius float64) []Aircraft {
	var inRange []Aircraft
	for _, a := range ac {
		dist := haversine(a.Latitude, a.Longitude, lat, long)
		if dist <= radius {
			inRange = append(inRange, a)
		}
	}
	return inRange
}

//func printLocalMilAircraft() {
//	milAircraft := getMilitaryAircraft()
//	// prints at my house's coords
//	aircraft := filterAircraftInRadius(milAircraft, 32.85336603690038, -97.41320921316995, 100)
//
//	for _, a := range aircraft {
//		fmt.Println(a.display())
//	}
//}
