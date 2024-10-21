package main

import (
	"common"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"text/template"
)

func fetchJSONData(url string, target interface{}) error {
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("status code error: %d %s", resp.StatusCode, resp.Status)
	}

	return json.NewDecoder(resp.Body).Decode(target)
}

func renderTemplate(w http.ResponseWriter, tmpl string, data interface{}) {
	w.Header().Set("Content-Type", "text/html")

	t, errTmpl := template.ParseFiles(tmpl)
	if errTmpl != nil {
		fmt.Fprintln(os.Stderr, errTmpl.Error())
		http.Error(w, "Error parsing template "+tmpl, http.StatusInternalServerError)
		return
	}

	if errExec := t.Execute(w, data); errExec != nil {
		fmt.Fprintln(os.Stderr, errExec.Error())
		return
	}
}

func Home(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		ErrorPage(w, http.StatusNotFound, "Page "+r.URL.Path[1:]+" Not Found.", "The page you requested was not found.")
		return
	}

	if len(r.URL.Query()) > 0 {
		ErrorPage(w, http.StatusBadRequest, "Invalid Request", "Home Page does not permit Queries.")
		return
	}

	groupieResp, errGGet := http.Get(ApiURL)
	if errGGet != nil {
		fmt.Fprintln(os.Stderr, errGGet.Error())
		ErrorPage(w, http.StatusInternalServerError, "Failed to fetch Groupie API", "There was a problem fetching "+ApiURL+".")
		return
	}
	defer groupieResp.Body.Close()

	if groupieResp.StatusCode != http.StatusOK {
		ErrorPage(w, http.StatusInternalServerError, "Failed to fetch Groupie API", "There was a problem fetching "+ApiURL+". Page returned code "+strconv.Itoa(groupieResp.StatusCode))
		return
	}

	var allInfos struct {
		Artists   []Artist
		Bounds    map[string]interface{}
		Locations []string
		Dates     []string
	} = struct {
		Artists   []Artist
		Bounds    map[string]interface{}
		Locations []string
		Dates     []string
	}{
		Artists:   ArtistsArray,
		Bounds:    getAllBounds(ArtistsArray),
		Locations: LocationsArray,
		Dates:     DatesArray,
	}

	renderTemplate(w, "templates/index.html", allInfos)
}

func Detail(w http.ResponseWriter, r *http.Request) {
	artistId := r.URL.Path[len("/detail/"):]
	id, errAtoi := strconv.Atoi(artistId)

	if errAtoi != nil {
		fmt.Fprintln(os.Stderr, errAtoi.Error())
		ErrorPage(w, http.StatusInternalServerError, "Not an ID", "Word "+artistId+" is not a valid ID.")
		return
	}
	if id <= 0 || id > len(ArtistsArray) {
		ErrorPage(w, http.StatusInternalServerError, "ID not valid", "ID "+artistId+" does not exist.")
		return
	}

	if len(ArtistsArray[id-1].Locations.LocationValues) == 0 || len(ArtistsArray[id-1].Relations.DatesLocations) == 0 {
		fmt.Println("Empty. Filling info for", ArtistsArray[id-1].Name+"...")
		var wg sync.WaitGroup

		wg.Add(2)
		go func() {
			defer wg.Done()
			var location Locations
			if errLJson := fetchJSONData(ArtistsArray[id-1].LocationsURL, &location); errLJson != nil {
				fmt.Fprintln(os.Stderr, errLJson.Error())
				ErrorPage(w, http.StatusInternalServerError, "Failed to fetch artist's Locations info.", "There was an error fetching "+ArtistsArray[id-1].LocationsURL+".")
			}
			ArtistsArray[id-1].Locations = location
		}()

		go func() {
			defer wg.Done()
			var rel Relation
			if errRJson := fetchJSONData(ArtistsArray[id-1].RelationsURL, &rel); errRJson != nil {
				fmt.Fprintln(os.Stderr, errRJson.Error())
				ErrorPage(w, http.StatusInternalServerError, "Failed to fetch artist's Relations info.", "There was an error fetching "+ArtistsArray[id-1].RelationsURL+".")
			}
			ArtistsArray[id-1].Relations = rel
		}()

		wg.Wait()

		fmt.Println("Done filling info for", ArtistsArray[id-1].Name+".")
	}

	if _, ok := InfoMap[ArtistsArray[id-1].Id]; !ok {
		var concerts []string
		var coords []LocationMap
		for _, loc := range ArtistsArray[id-1].Locations.LocationValues {
			formattedLoc := FormatLocation(loc)
			concerts = append(concerts, formattedLoc+": "+strings.Replace(strings.Join(ArtistsArray[id-1].Relations.DatesLocations[loc], ", "), "*", "", -1))
			coords = append(coords, LocationMap{
				Name:      formattedLoc,
				Longitude: AllLocations[formattedLoc].Longitude,
				Latitude:  AllLocations[formattedLoc].Latitude,
			})
		}

		InfoMap[ArtistsArray[id-1].Id] = ArtistInfo{
			TheArtist:     ArtistsArray[id-1],
			TheirConcerts: concerts,
			TheirCoords:   coords,
		}
	}

	renderTemplate(w, "templates/detail.html", InfoMap[ArtistsArray[id-1].Id])
}

func Filter(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if len(r.URL.Query()) == 0 {
		json.NewEncoder(w).Encode("Empty filter query forbidden")
		return
	}

	filteredArtists := make([]map[string]interface{}, 0)

	for _, artist := range ArtistsArray {
		if matchesFilters(artist, r.URL.Query().Get("membs"), r.URL.Query().Get("minCrtD"), r.URL.Query().Get("maxCrtD"), r.URL.Query().Get("minFAD"), r.URL.Query().Get("maxFAD"), r.URL.Query().Get("location"), r.URL.Query().Get("date")) {
			filteredArtists = append(filteredArtists, createArtistResult(artist))
		}
	}

	json.NewEncoder(w).Encode(filteredArtists)
}

func Search(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("q")
	if query == "" {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(SearchArtists(query, ArtistsArray))
}

func SearchArtists(query string, arr []Artist) []map[string]interface{} {
	query = strings.ToLower(query)
	var searchResults []map[string]interface{}

	for _, artist := range ArtistsArray {
		found := false
		if strings.Contains(strings.ToLower(artist.Name), query) ||
			strings.Contains(strings.ToLower(strings.Join(artist.Members, " ")), query) ||
			strings.Contains(strings.ToLower(artist.FirstAlbum), query) ||
			strings.Contains(fmt.Sprint(artist.CreationDate), query) {
			searchResults = append(searchResults, createArtistResult(artist))
			continue
		}

		for _, loc := range artist.Locations.LocationValues {
			if strings.Contains(loc, strings.ToLower(query)) {
				searchResults = append(searchResults, createArtistResult(artist))
				found = true
				break
			}
		}
		if found {
			continue
		}

		for _, date := range artist.ConcertDates.DatesValues {
			if strings.Contains(date, query) {
				searchResults = append(searchResults, createArtistResult(artist))
				break
			}
		}
	}

	return searchResults
}

func ErrorPage(w http.ResponseWriter, code int, msg, desc string) {
	renderTemplate(w, "templates/error.html", ErrorData{
		Code:        code,
		Message:     msg,
		Description: desc,
	})
}

func createArtistResult(artist Artist) map[string]interface{} {
	return map[string]interface{}{
		"type": "artist/band",
		"band": artist,
	}
}

func getAllBounds(arr []Artist) map[string]interface{} {
	var maxMemb uint
	minCrtD, maxCrtD := uint(9999), uint(0)
	minFAD, maxFAD := "9999-99-99", "0000-00-00"

	for _, artist := range arr {
		if len(artist.Members) > int(maxMemb) {
			maxMemb = uint(len(artist.Members))
		}

		if artist.CreationDate < minCrtD {
			minCrtD = artist.CreationDate
		}

		if artist.CreationDate > maxCrtD {
			maxCrtD = artist.CreationDate
		}

		if strings.Compare(common.StrWordRev(artist.FirstAlbum), minFAD) < 0 {
			minFAD = common.StrWordRev(artist.FirstAlbum)
		}

		if strings.Compare(common.StrWordRev(artist.FirstAlbum), maxFAD) > 0 {
			maxFAD = common.StrWordRev(artist.FirstAlbum)
		}
	}

	amtMemb := make([]uint, 0)
	for i := 1; i <= int(maxMemb); i++ {
		amtMemb = append(amtMemb, uint(i))
	}

	return map[string]interface{}{
		"amtMemb": amtMemb,
		"minCrtD": minCrtD,
		"maxCrtD": maxCrtD,
		"minFAD":  minFAD,
		"maxFAD":  maxFAD,
	}
}

func matchesFilters(a Artist, allMembAmts, minCrt, maxCrt, minFrstAlb, maxFrstAlb, location, date string) bool {
	membsArray := []uint{}

	if allMembAmts != "" {
		for _, val := range strings.Split(allMembAmts, ",") {
			amt, err := strconv.Atoi(val)
			if err != nil {
				return false
			}
			membsArray = append(membsArray, uint(amt))
		}

		if common.IndexOf(membsArray, uint(len(a.Members))) == -1 {
			return false
		}
	}

	if minCrt != "" {
		date, err := strconv.Atoi(minCrt)
		if err != nil {
			return false
		}
		if a.CreationDate < uint(date) {
			return false
		}
	}

	if maxCrt != "" {
		date, err := strconv.Atoi(maxCrt)
		if err != nil {
			return false
		}
		if a.CreationDate > uint(date) {
			return false
		}
	}

	if minFrstAlb != "" {
		revDate := common.StrWordRev(a.FirstAlbum)
		if strings.Compare(revDate, minFrstAlb) < 0 {
			return false
		}
	}

	if maxFrstAlb != "" {
		revDate := common.StrWordRev(a.FirstAlbum)
		if strings.Compare(revDate, maxFrstAlb) > 0 {
			return false
		}
	}

	if location != "" {
		found := false
		for _, loc := range a.Locations.LocationValues {
			formattedLoc := FormatLocation(loc)
			if formattedLoc == location {
				found = true
			}
		}
		if !found {
			return false
		}
	}

	if date != "" {
		found := false
		for _, dateVal := range a.ConcertDates.DatesValues {
			revDate := common.StrWordRev(dateVal)
			if revDate[len(revDate)-1] == '*' {
				revDate = revDate[:len(revDate)-1]
			}
			if revDate == strings.Replace(date, "/", "-", 2) {
				found = true
			}
		}
		if !found {
			return false
		}
	}

	return true
}
