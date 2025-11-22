package main

import (
	"fmt"
	"io"
	"net/http"
	"scrap/output"
	scraper "scrap/parser"

	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

///////////////////////////////////////////////////////////////////////////////
// SECURITY — sanitize and validate package name
///////////////////////////////////////////////////////////////////////////////

func sanitizePackage(pkg string) (string, error) {

	pkg = strings.TrimSpace(pkg)
	pkg = strings.ToLower(pkg)

	if pkg == "" {
		return "", fmt.Errorf("package name is required")
	}

	if len(pkg) > 60 {
		return "", fmt.Errorf("package name too long")
	}

	if !strings.Contains(pkg, ".") {
		return "", fmt.Errorf("invalid package format (use com.example.app)")
	}

	if strings.ContainsAny(pkg, "/\\?*&=<>'\"{}()[]|;: ") {
		return "", fmt.Errorf("unsafe characters detected")
	}

	for _, c := range pkg {
		if !(c >= 'a' && c <= 'z') &&
			!(c >= '0' && c <= '9') &&
			c != '.' {
			return "", fmt.Errorf("invalid character in package name")
		}
	}

	return pkg, nil
}

///////////////////////////////////////////////////////////////////////////////
// SCALABLE CACHE — Thread-Safe with Expiry
///////////////////////////////////////////////////////////////////////////////

type CacheEntry struct {
	Data      *scraper.App
	Timestamp int64
}

var Cache = make(map[string]CacheEntry)
var cacheLock = &sync.RWMutex{}

const CacheTTL = 6 * 60 * 60 // 6 hours

func getFromCache(pkg string) (*scraper.App, bool) {
	cacheLock.RLock()
	entry, found := Cache[pkg]
	cacheLock.RUnlock()

	if !found {
		return nil, false
	}

	if time.Now().Unix()-entry.Timestamp > CacheTTL {
		cacheLock.Lock()
		delete(Cache, pkg)
		cacheLock.Unlock()
		return nil, false
	}

	return entry.Data, true
}

func saveToCache(pkg string, app *scraper.App) {
	cacheLock.Lock()
	Cache[pkg] = CacheEntry{
		Data:      app,
		Timestamp: time.Now().Unix(),
	}
	cacheLock.Unlock()
}

///////////////////////////////////////////////////////////////////////////////
// MAIN SERVER
///////////////////////////////////////////////////////////////////////////////

func main() {

	r := gin.Default()
	r.LoadHTMLGlob("templates/*")

	//-----------------------------------------------------------------------
	// HOME PAGE
	//-----------------------------------------------------------------------
	r.GET("/", func(c *gin.Context) {
		c.HTML(200, "index.html", nil)
	})

	//-----------------------------------------------------------------------
	// APP INFO ROUTE — security + caching + retry + scalability
	//-----------------------------------------------------------------------
	r.GET("/app-info", func(c *gin.Context) {

		raw := c.Query("package")

		// SECURITY
		pkg, err := sanitizePackage(raw)
		if err != nil {
			output.ShowErrorPage(c, err.Error())
			return
		}

		// CACHE CHECK
		if app, ok := getFromCache(pkg); ok {
			fmt.Println("CACHE HIT:", pkg)
			output.ShowAppInfo(c, app)
			return
		}

		// RETRY SCRAPER (3 attempts)
		var app *scraper.App
		for retry := 1; retry <= 3; retry++ {

			url := "https://play.google.com/store/apps/details?id=" + pkg

			app, err = scraper.ScrapePlayStoreByURL(url) // ⭐ FULL CHROMEDP SCRAPER
			if err == nil {
				break
			}

			fmt.Println("Retry:", retry, "for", pkg)
			time.Sleep(time.Second)
		}

		if err != nil {
			output.ShowErrorPage(c, "Failed to reach Google Play. Try again.")
			return
		}

		// SAVE TO CACHE
		saveToCache(pkg, app)
		fmt.Println("CACHE SAVED:", pkg)

		// DISPLAY RESULT
		output.ShowAppInfo(c, app)
	})

	r.GET("/proxy", func(c *gin.Context) {
		url := c.Query("url")
		if url == "" {
			c.Status(400)
			return
		}

		resp, err := http.Get(url)
		if err != nil {
			c.Status(500)
			return
		}
		defer resp.Body.Close()

		c.Header("Content-Type", resp.Header.Get("Content-Type"))
		io.Copy(c.Writer, resp.Body)
	})

	r.Run(":8080")
}
