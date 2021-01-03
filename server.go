package main

import (
	"fmt"
	"log"
	"math"
	"os"
	"strconv"
	"strings"

	"github.com/blevesearch/bleve"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/template/html"
	"github.com/ilyakaznacheev/cleanenv"
	"github.com/qinains/fastergoding"
)

func main() {
	fastergoding.Run() // hot reload
	var cfg Config

	homeDir, err := os.UserHomeDir()
	if err != nil {
		log.Fatal("Error retrieving user home dir")
	}
	if err = cleanenv.ReadConfig(homeDir+"/coreander/coreander.yml", &cfg); err != nil {
		log.Fatal(fmt.Sprintf("Config file coreander.yml not found in %s/coreander", homeDir))
	}
	idx, err := bleve.Open(homeDir + "/coreander/coreander.db")
	if err == bleve.ErrorIndexPathDoesNotExist {
		log.Println("No index found, creating a new one")
		idx, err = create()
		if err != nil {
			log.Fatal(err)
		}
		err = add(idx, cfg.LibraryPath)
		if err != nil {
			log.Fatal(err)
		}
	}
	engine := html.New("./views", ".html").Reload(true).Debug(true)
	app := fiber.New(fiber.Config{
		Views: engine,
	})

	app.Get("/", func(c *fiber.Ctx) error {
		keywords := c.Query("search")
		page, err := strconv.Atoi(c.Query("page"))
		if err != nil {
			page = 1
		}
		if page < 1 {
			page = 1
		}
		if keywords != "" {
			query := bleve.NewMatchQuery(keywords)
			search := bleve.NewSearchRequestOptions(query, 10, (page-1)*10, false)
			search.Fields = []string{"Title", "Author", "Description"}
			searchResults, _ := idx.Search(search)
			if searchResults.Total < uint64(page-1)*10 {
				page = 1
				search = bleve.NewSearchRequestOptions(query, 10, (page-1)*10, false)
				search.Fields = []string{"Title", "Author", "Description"}
				searchResults, _ = idx.Search(search)
			}
			//fmt.Println(searchResults.Hits[0].Fields["Author"].(string))
			// PARCHE
			for _, v := range searchResults.Hits {
				v.ID = strings.Replace(v.ID, cfg.LibraryPath, "", 1)
			}
			pages := int(math.Ceil(float64(searchResults.Total) / float64(10)))
			idx.Search(search)
			return c.Render("results", fiber.Map{
				"Keywords":  keywords,
				"Results":   searchResults.Hits,
				"Total":     searchResults.Total,
				"Paginator": pagination(10, pages, page, keywords),
			}, "layout")
		}
		return c.Render("index", fiber.Map{}, "layout")
	})

	app.Static("/files", cfg.LibraryPath)
	app.Static("/css", "./css")
	app.Listen(cfg.Port)
}
