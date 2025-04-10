package crawler

import (
	"database/sql"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"
)

func Start(workers int, db *sql.DB) {
	fmt.Println("Starting crawler...")

	var count int
	var specialities []Speciality
	var depatments []Department
	var cities []City

	err := db.QueryRow(`SELECT COUNT(*) FROM specialities`).Scan(&count)
	if err != nil {
		log.Printf("Failed to check specialities count: %v", err)
		return
	}
	if count > 0 {
		fmt.Println("Records already exists in DB, retrieving it ...")
		rows, err := db.Query(`SELECT nom, url FROM specialities`)
		if err != nil {
			log.Fatalf("Failed to query specialities: %v", err)
		}
		defer rows.Close()
		for rows.Next() {
			var s Speciality
			if err := rows.Scan(&s.Nom, &s.URL); err != nil {
				log.Printf("Failed to scan speciality: %v", err)
				continue
			}
			specialities = append(specialities, s)
		}
		fmt.Printf("[INFO] Found %d records in speciality\n", len(specialities))
	} else {
		specialities = GetSpecialities(db)
	}

	count = 0
	err = db.QueryRow(`SELECT COUNT(*) FROM departments`).Scan(&count)
	if err != nil {
		log.Printf("Failed to check department count: %v", err)
		return
	}

	if count > 0 {
		fmt.Println("Records already exists in DB, retrieving it ...")
		rows, err := db.Query(`SELECT nom, url, code, speciality FROM departments`)
		if err != nil {
			log.Fatalf("Failed to query departments: %v", err)
		}
		defer rows.Close()
		for rows.Next() {
			var d Department
			if err := rows.Scan(&d.Name, &d.URL, &d.Code, &d.Speciality); err != nil {
				log.Printf("Failed to scan department: %v", err)
				continue
			}
			depatments = append(depatments, d)
		}
		fmt.Printf("[INFO] Found %d records in departments\n", len(depatments))
	} else {
		for _, s := range specialities {
			deps := GetDepartments(s, db)
			depatments = append(depatments, deps...)
			fmt.Printf("[DEBUG] Found %d department for %s speciality\n", len(deps), s.Nom)
		}
	}

	count = 0
	err = db.QueryRow(`SELECT COUNT(*) FROM cities`).Scan(&count)
	if err != nil {
		log.Printf("Failed to check cities count: %v", err)
		return
	}

	if count > 0 {
		fmt.Println("Records already exists in DB, retrieving it ...")
		rows, err := db.Query(`SELECT nom, url FROM cities`)
		if err != nil {
			log.Fatalf("Failed to query cities: %v", err)
		}
		defer rows.Close()
		for rows.Next() {
			var c City
			if err := rows.Scan(&c.Name, &c.URL); err != nil {
				log.Printf("Failed to scan cities: %v", err)
				continue
			}
			cities = append(cities, c)
		}
		fmt.Printf("[INFO] Found %d records in cities\n", len(cities))
	} else {
		for _, d := range depatments {
			cits := GetCities(d, db)
			cities = append(cities, cits...)
			fmt.Printf("[DEBUG] Found %d cities for %s department and speciliaty %s\n", len(cits), d.Code, d.Speciality)
		}
	}

	jobs := make(chan CrawlJob, 100)

	wg := &sync.WaitGroup{}

	for i := 0; i < workers; i++ {
		go worker(jobs, wg, i)

	}
	// go createJobs(jobs, wg, depatments)

	go func() {
		wg.Wait()
		close(jobs)
	}()

}

func createJobs(jobs chan<- CrawlJob, wg *sync.WaitGroup, departments []Department) {
	// for _, d := range departments {
	// 	cities := GetCities(d)

	// 	for _, c := range cities {
	// 		wg.Add(1)
	// 		jobs <- CrawlJob{Department: d, City: c}
	// 	}

	// }
}

func worker(jobs <-chan CrawlJob, wg *sync.WaitGroup, id int) {
	for job := range jobs {
		fmt.Printf("[DEBUG] Worker %d starting - %s, %s, %s\n", id, job.Speciality.Nom, job.Department.Code, job.City.Name)

		details := GetDetails(job.Speciality, job.Department, job.City)
		for _, url := range details {
			doc := GetDoctor(url)

			fmt.Println(doc)
		}
		fmt.Printf("[DEBUG] Worker %d done", id)
		wg.Done()
	}
}

type CrawlJob struct {
	Speciality Speciality
	Department Department
	City       City
}

type Speciality struct {
	Nom string
	URL string
}

type Department struct {
	Speciality string `json:"speciality"`
	Name       string `json:"name"`
	Code       string `json:"code"`
	URL        string `json:"url"`
}

type City struct {
	Name       string `json:"name"`
	URL        string `json:"url"`
	Speciality string `json:"speciality"`
	Code       string `json:"code"`
}

type Doctor struct {
	Nom        string
	Speciality string
	Adresse    string
	CodePostal string
	Ville      string
	Telephone  string
	UrlFiche   string
}

func GetSpecialities(db *sql.DB) []Speciality {
	body, err := getUrl("https://annuairesante.ameli.fr/trouver-un-professionnel-de-sante/")

	if err != nil {
		return nil
	}

	err = os.WriteFile("debug_output.html", []byte(body), os.ModeAppend|0644)
	if err != nil {
		log.Printf("Failed to write to file: %v", err)
	} else {
		fmt.Println("Trimmed HTML appended to debug_output.html")
	}

	// It should be this regex but they are bad at html
	// ulRegex := regexp.MustCompile(`(?ms)<ul class="(first|second|third)">.*?</ul>`)
	ulRegex := regexp.MustCompile(`<ul class=('first'|second|third)>.*?</ul>`)
	fmt.Println(ulRegex.FindAllString(body, -1))
	aTagRegex := regexp.MustCompile(`<a href="(.*?)">(.*?)</a>`)

	var specialities []Speciality

	ulMatches := ulRegex.FindAllString(body, -1)
	for _, ul := range ulMatches {

		aTags := aTagRegex.FindAllStringSubmatch(ul, -1)
		for _, match := range aTags {
			href := match[1]
			name := htmlUnescape(match[2])
			fullURL := "https://annuairesante.ameli.fr" + href
			specialities = append(specialities, Speciality{
				Nom: name,
				URL: fullURL,
			})
			_, err := db.Exec(`INSERT OR IGNORE INTO specialities(nom, url) VALUES (?, ?)`, name, fullURL)
			if err != nil {
				log.Printf("Insert failed for %s: %v", name, err)
			}
		}
	}

	return specialities
}

func GetDepartments(s Speciality, db *sql.DB) []Department {
	body, err := getUrl(s.URL)
	if err != nil {
		return nil
	}
	aTagRegex := regexp.MustCompile(`<li class="seo-departement">\s*<a href="(.*?)">`)

	var departements []Department

	// fmt.Println(ulMatches)
	aTags := aTagRegex.FindAllStringSubmatch(body, -1)
	for _, match := range aTags {
		// fmt.Println(match)
		href := match[1]
		parts := strings.Split(href, "/")
		name := parts[len(parts)-1]
		code := strings.SplitN(name, "-", 2)[0]
		name = strings.SplitN(name, "-", 2)[1]
		fullURL := "https://annuairesante.ameli.fr" + href
		speciality := parts[2]
		departements = append(departements, Department{
			Speciality: speciality,
			Name:       name,
			Code:       code,
			URL:        fullURL,
		})
		_, err := db.Exec(`INSERT OR IGNORE INTO departments(nom, url, code, speciality) VALUES (?, ?, ?, ?)`, name, fullURL, code, speciality)
		if err != nil {
			log.Printf("Insert failed for %s: %v", name, err)
		}
	}

	return departements
}

func GetCities(d Department, db *sql.DB) []City {
	body, err := getUrl(d.URL)

	if err != nil {
		return nil
	}

	ulRegex := regexp.MustCompile(`<ul class=('first'|second|third)>.*?</ul>`)
	// fmt.Println(ulRegex.FindAllString(body, -1))
	aTagRegex := regexp.MustCompile(`<a href="(.*?)">(.*?)</a>`)

	var cities []City

	ulMatches := ulRegex.FindAllString(body, -1)
	for _, ul := range ulMatches {

		aTags := aTagRegex.FindAllStringSubmatch(ul, -1)
		for _, match := range aTags {
			href := match[1]
			name := htmlUnescape(match[2])
			fullURL := "https://annuairesante.ameli.fr" + href
			cities = append(cities, City{
				Name:       name,
				URL:        fullURL,
				Speciality: d.Speciality,
				Code:       d.Code,
			})
			// fmt.Printf("[DEBUG] %s %s %s %s\n", name, fullURL, d.Speciality, d.Code)
			_, err := db.Exec(`INSERT OR IGNORE INTO cities(nom, url, code, speciality) VALUES (?, ?, ?, ?)`, name, fullURL, d.Speciality, d.Code)
			if err != nil {
				log.Printf("Insert failed for %s: %v", name, err)
			}
		}
	}

	return cities
}

func GetDetails(s Speciality, d Department, c City) []string {
	return []string{}
}

func GetDoctor(url string) Doctor {
	time.Sleep(100 * time.Millisecond)
	return Doctor{}
}

func getUrl(url string) (body string, error error) {
	resp, err := http.Get(url)
	if err != nil {
		log.Printf("Failed to fetch URL: %v", err)
		return body, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		log.Printf("Received non-200 response code: %d", resp.StatusCode)
		return body, err
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("Failed to read body: %v", err)
		return body, err
	}
	html := string(bodyBytes)
	body = htmlTrim(html)
	return body, nil
}

func htmlTrim(html string) string {
	// Remove all tabs, newlines, and carriage returns
	html = strings.ReplaceAll(html, "\n", "")
	html = strings.ReplaceAll(html, "\t", "")
	html = strings.ReplaceAll(html, "\r", "")

	// Replace multiple spaces with one
	spaceRegex := regexp.MustCompile(`\s{2,}`)
	html = spaceRegex.ReplaceAllString(html, " ")

	// Remove space between tags: >   < => ><
	tagSpaceRegex := regexp.MustCompile(`>\s+<`)
	html = tagSpaceRegex.ReplaceAllString(html, "><")

	return strings.TrimSpace(html)
}

func htmlUnescape(s string) string {
	replacer := strings.NewReplacer(
		"&amp;", "&",
		"&lt;", "<",
		"&gt;", ">",
		"&quot;", `"`,
		"&#39;", "'",
	)
	return replacer.Replace(s)
}
