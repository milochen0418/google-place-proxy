package proxy

import (
    "appengine"
    "appengine/memcache"
    "appengine/urlfetch"
    "encoding/json"
    "errors"
    "fmt"
    "io/ioutil"
    "net/http"
   "net/url"
    "strconv"
    "strings"
    "time"
    "io"
)

// New constants for location rounding and cache expiry.
const locationPrecision = 4  // Up to 11 metres
const cacheExpiry = time.Second * 600  // 5 minutes
//const placesAPIKey = "AIzaSyB4ic2rJgFx8kwzFbQfwzGMmZohHxnlc74"  //<-- key in core-outcome-555
const placesAPIKey = "AIzaSyCTvmQq--hEjslW2pKT1BqSkfiPrNUWeD0" //<-- api key in freemium-dev
//const placesAPIKey = "AIzaSyAKhQQUyfSSK7i0uEjoTg9YhFvB9Qhd6jA" //<-- api key in milo@pebblar.com


const placesURL = "https://maps.googleapis.com/maps/api/place/nearbysearch/json?key=%s&location=%s&radius=%s"


const placeAutocompleteGlobalURL = 	"https://maps.googleapis.com/maps/api/place/autocomplete/json?key=%s&input=%s"
//Original Google Query
//https://maps.googleapis.com/maps/api/place/autocomplete/json?input=Mel&key=AIzaSyBm7eiOmjgp-LHVQf6u4ujXbE9xaMUBXgc


const placeAutocompleteRadiusURL = 	"https://maps.googleapis.com/maps/api/place/autocomplete/json?key=%s&input=%s&location=%s&radius=%s"
//Original Google Query
//https://maps.googleapis.com/maps/api/place/autocomplete/json?input=Mel&location=20.6556568,-103.2968223&radius=10000&key=AIzaSyBm7eiOmjgp-LHVQf6u4ujXbE9xaMUBXgc 


const placeDetailsURL = 				"https://maps.googleapis.com/maps/api/place/details/json?key=%s&placeid=%s"
//Original Google Query
//https://maps.googleapis.com/maps/api/place/details/json?placeid=EjxDYWxsZSBBZHJpw6FuIFB1Z2EsIFNhbiBSYWZhZWwsIEd1YWRhbGFqYXJhLCBKYWxpc2NvLCBNZXhpY28iLiosChQKEgktjGaY6bMohBG1aAULAoybrBIUChIJvdm4asSzKIQRwU8CkvMFaUY&key=AIzaSyBm7eiOmjgp-LHVQf6u4ujXbE9xaMUBXgc 



type placeResults struct {
    Results []struct {
        Geometry struct {
            Location struct {
                Lat float64 `json:"lat"`
                Lng float64 `json:"lng"`
            } `json:"location"`
        } `json:"geometry"`
    } `json:"results"`
}

// Rounds off the latitude and longitude of a location.
func normalizeLocation(location string) (string, error) {
    var lat, lng float64
    var err error
    latLng := strings.Split(location, ",")
    if len(latLng) != 2 {
        return "", errors.New("Invalid location")
    }
    if lat, err = strconv.ParseFloat(latLng[0], locationPrecision); err != nil {
        return "", errors.New("Invalid location")
    }
    if lng, err = strconv.ParseFloat(latLng[1], locationPrecision); err != nil {
        return "", errors.New("Invalid location")
    }
    return fmt.Sprintf("%.2f,%.2f", lat, lng), nil
}

func formatPlaces(body io.Reader)([]byte, error) {
    var places placeResults
    if err := json.NewDecoder(body).Decode(&places); err != nil {
        return nil, err
    }
    return json.Marshal(places)
}

// fetchPlaces now stores results in the cache.
func fetchPlaces(ctx appengine.Context, location, radius string) ([]byte, error) {
    client := urlfetch.Client(ctx)
    resp, err := client.Get(fmt.Sprintf(placesURL, placesAPIKey, location, radius))
    if err != nil {
        return nil, err
    }
    defer resp.Body.Close()
    places, err := formatPlaces(resp.Body)
    if (err == nil) {
        memcache.Set(ctx, &memcache.Item{
            Key:        location,
            Value:      places,
            Expiration: cacheExpiry,
        })
    }
    return places, err
}




func setupResponse(w *http.ResponseWriter, req *http.Request) {
	(*w).Header().Set("Access-Control-Allow-Origin", "*")
    (*w).Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS, PUT, DELETE")
    (*w).Header().Set("Access-Control-Allow-Headers", "Accept, Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization")
}

func fetchOrigPlaces(ctx appengine.Context, location, radius string) ([]byte, error) {
    client := urlfetch.Client(ctx)
    resp, err := client.Get(fmt.Sprintf(placesURL, placesAPIKey, location, radius))
    if err != nil {
        return nil, err
    }
    defer resp.Body.Close()
    return ioutil.ReadAll(resp.Body)
}

func testOrigHandler(w http.ResponseWriter, r *http.Request) {
	setupResponse(&w, r)
    ctx := appengine.NewContext(r)
    places, err := fetchOrigPlaces(ctx, r.FormValue("location"), r.FormValue("radius"))
    if err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }
    w.Header().Add("Content-Type", "application/json; charset=utf-8")
    w.Write(places)	
}

// handler now retrieves results from the cache if they exist.
func testHandler(w http.ResponseWriter, r *http.Request) {
	
	setupResponse(&w, r)
    radius := r.FormValue("radius")
    location, err := normalizeLocation(r.FormValue("location"))
    if err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }
    ctx := appengine.NewContext(r)
    var places []byte
    if cached, err := memcache.Get(ctx, location); err == nil {
        places = cached.Value
        // We use Golang's goroutines here to call fetchPlaces in the background, 
        // without having to wait for the result it returns. This ensures that 
        // both the cache remains fresh, and that we also remain in compliance 
        // with the Google Places API Policies.
        go fetchPlaces(ctx, location, radius)
    } else if places, err = fetchPlaces(ctx, location, radius); err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }
    w.Header().Add("Content-Type", "application/json; charset=utf-8")
    w.Write(places)
}




/*
func fetchOrigPlaces(ctx appengine.Context, location, radius string) ([]byte, error) {
    client := urlfetch.Client(ctx)
    resp, err := client.Get(fmt.Sprintf(placesURL, placesAPIKey, location, radius))
    if err != nil {
        return nil, err
    }
    defer resp.Body.Close()
    return ioutil.ReadAll(resp.Body)
}

func testOrigHandler(w http.ResponseWriter, r *http.Request) {
	setupResponse(&w, r)
    ctx := appengine.NewContext(r)
    places, err := fetchOrigPlaces(ctx, r.FormValue("location"), r.FormValue("radius"))
    if err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }
    w.Header().Add("Content-Type", "application/json; charset=utf-8")
    w.Write(places)	
}
*/


func fetchDetails(ctx appengine.Context, placeid string) ([]byte, error) {
    client := urlfetch.Client(ctx)
    resp, err := client.Get(fmt.Sprintf(placeDetailsURL, placesAPIKey, placeid))
    if err != nil {
        return nil, err
    }
    defer resp.Body.Close()
    return ioutil.ReadAll(resp.Body)
}


func placeDetailsHandler(w http.ResponseWriter, r *http.Request) {
	/**	
	const placeDetailsURL = 				"https://maps.googleapis.com/maps/api/place/details/json?key=%s&placeid=%s"
	//Original Google Query
	//https://maps.googleapis.com/maps/api/place/details/json?placeid=EjxDYWxsZSBBZHJpw6FuIFB1Z2EsIFNhbiBSYWZhZWwsIEd1YWRhbGFqYXJhLCBKYWxpc2NvLCBNZXhpY28iLiosChQKEgktjGaY6bMohBG1aAULAoybrBIUChIJvdm4asSzKIQRwU8CkvMFaUY&key=AIzaSyBm7eiOmjgp-LHVQf6u4ujXbE9xaMUBXgc 
	//TEST https://place-proxy.appspot.com/details?placeid=EjxDYWxsZSBBZHJpw6FuIFB1Z2EsIFNhbiBSYWZhZWwsIEd1YWRhbGFqYXJhLCBKYWxpc2NvLCBNZXhpY28iLiosChQKEgktjGaY6bMohBG1aAULAoybrBIUChIJvdm4asSzKIQRwU8CkvMFaUY
	**/
	setupResponse(&w, r)
    ctx := appengine.NewContext(r)
    places, err := fetchDetails(ctx, r.FormValue("placeid"))
    if err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }
    w.Header().Add("Content-Type", "application/json; charset=utf-8")
    w.Write(places)	
}





func fetchAutocompleteRadius(ctx appengine.Context, input string, location string, radius string) ([]byte, error) {
    client := urlfetch.Client(ctx)
    resp, err := client.Get(fmt.Sprintf(placeAutocompleteRadiusURL, placesAPIKey, input, location, radius))
    if err != nil {
        return nil, err
    }
    defer resp.Body.Close()
    return ioutil.ReadAll(resp.Body)
}


func fetchAutocompleteGlobal(ctx appengine.Context, input string) ([]byte, error) {
    client := urlfetch.Client(ctx)
    //const placeAutocompleteGlobalURL = 	"https://maps.googleapis.com/maps/api/place/autocomplete/json?key=%s&input=%s"
    //resp, err := client.Get(fmt.Sprintf(placeAutocompleteGlobalURL, placesAPIKey, input))
    
    u, _ := url.Parse("https://maps.googleapis.com/maps/api/place/autocomplete/json")
    q := u.Query()
    q.Set("key",placesAPIKey)
    q.Set("search",input)
    u.RawQuery = q.Encode()
    resp, err := client.Get(fmt.Sprintf(placeAutocompleteGlobalURL, placesAPIKey, input))
    //fmt.Println("Hello World")
    //resp, err := client.Get(u.String())
    
    if err != nil {
        return nil, err
    }
    defer resp.Body.Close()
    return ioutil.ReadAll(resp.Body)
}


//search, search and location 

func placeAutocompleteHandler(w http.ResponseWriter, r *http.Request) {
	/**
	const placeAutocompleteGlobalURL = 	"https://maps.googleapis.com/maps/api/place/autocomplete/json?key=%s&input=%s"
	//Original Google Query
	//https://maps.googleapis.com/maps/api/place/autocomplete/json?input=Mel&key=AIzaSyBm7eiOmjgp-LHVQf6u4ujXbE9xaMUBXgc
	//TEST https://place-proxy.appspot.com/autocomplete?input=Mel
	
	const placeAutocompleteRadiusURL = 	"https://maps.googleapis.com/maps/api/place/autocomplete/json?key=%s&input=%s&location=%s&radius=%s"
	//Original Google Query
	//https://maps.googleapis.com/maps/api/place/autocomplete/json?input=Mel&location=20.6556568,-103.2968223&radius=10000&key=AIzaSyBm7eiOmjgp-LHVQf6u4ujXbE9xaMUBXgc 
	//TEST https://place-proxy.appspot.com/autocomplete/json?input=Mel&radius=10000&location=20.6556568,-103.2968223
	**/
		
	
	input := r.FormValue("input")
    radius := r.FormValue("radius")
    
    if radius != "" {
    	location := r.FormValue("location")
		//autocomplete with parameters of input, radius, location
    	setupResponse(&w, r)		
	    ctx := appengine.NewContext(r)
	    
	    places, err := fetchAutocompleteRadius(ctx, input, location, radius)
	    if err != nil {
	        http.Error(w, err.Error(), http.StatusInternalServerError)
	        return
	    }
	    w.Header().Add("Content-Type", "application/json; charset=utf-8")
	    w.Write(places)	 
	    return 
    } 
    

	//autocomplete with parameters of input
	setupResponse(&w, r)
    ctx := appengine.NewContext(r)
    places, err := fetchAutocompleteGlobal(ctx, input)
    if err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }
    w.Header().Add("Content-Type", "application/json; charset=utf-8")
    w.Write(places)	 
}


/* searchAutocompleteHandler is compatible with original Ming's backend autocomplete API design for two kind of parameters location and search   */
func searchAutocompleteHandler(w http.ResponseWriter, r *http.Request) {
	/**
	const placeAutocompleteGlobalURL = 	"https://maps.googleapis.com/maps/api/place/autocomplete/json?key=%s&input=%s"
	//Original Google Query
	//https://maps.googleapis.com/maps/api/place/autocomplete/json?input=Mel&key=AIzaSyBm7eiOmjgp-LHVQf6u4ujXbE9xaMUBXgc
	//TEST https://place-proxy.appspot.com/searchAutocomplete?search=Mel
	
	const placeAutocompleteRadiusURL = 	"https://maps.googleapis.com/maps/api/place/autocomplete/json?key=%s&input=%s&location=%s&radius=%s"
	//Original Google Query
	//https://maps.googleapis.com/maps/api/place/autocomplete/json?input=Mel&location=20.6556568,-103.2968223&radius=10000&key=AIzaSyBm7eiOmjgp-LHVQf6u4ujXbE9xaMUBXgc 
	//TEST https://place-proxy.appspot.com/searchAutocomplete?search=Mel&location=20.6556568,-103.2968223
	**/	
	
	input := r.FormValue("search")
    location := r.FormValue("location")
    
    if location != "" {
    	radius := "10000"
		//autocomplete with parameters of input, radius, location
    	setupResponse(&w, r)		
	    ctx := appengine.NewContext(r)
	    
	    places, err := fetchAutocompleteRadius(ctx, input, location, radius)
	    if err != nil {
	        http.Error(w, err.Error(), http.StatusInternalServerError)
	        return
	    }
	    w.Header().Add("Content-Type", "application/json; charset=utf-8")
	    w.Write(places)	 
	    return 
    } 

	//autocomplete with parameters of input
	setupResponse(&w, r)
    ctx := appengine.NewContext(r)
    places, err := fetchAutocompleteGlobal(ctx, input)
    if err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }
    w.Header().Add("Content-Type", "application/json; charset=utf-8")
    w.Write(places)	 
}



func faviconHandler(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, "./favicon.ico")
}

func init() {
	http.HandleFunc("/favicon.ico", faviconHandler)	
    http.HandleFunc("/", testHandler)
    http.HandleFunc("/origin", testOrigHandler)
    http.HandleFunc("/autocomplete", placeAutocompleteHandler)
    http.HandleFunc("/details", placeDetailsHandler)
    http.HandleFunc("/searchAutocomplete", searchAutocompleteHandler)
}
