package output

import (
	"fmt"
	"net/http"

	scraper "scrap/parser"

	"github.com/gin-gonic/gin"
)

// ShowErrorPage displays an error message
func ShowErrorPage(c *gin.Context, message string) {
	html := fmt.Sprintf(`
		<h2 style="color:red;">%s</h2>
		<a href="/">⬅ Go Back</a>
	`, message)
	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(html))
}

// ShowAppInfo displays full Play Store info
func ShowAppInfo(c *gin.Context, app *scraper.App) {

	rating := "N/A"
	if app.Rating != "" {
		rating = app.Rating
	}

	ratingCount := "N/A"
	if app.RatingCount != "" {
		ratingCount = app.RatingCount
	}

	// Build screenshot gallery
	screensHTML := ""
	if len(app.Screenshots) > 0 {
		for _, img := range app.Screenshots {
			screensHTML += fmt.Sprintf(
				`<img src="%s" width="160" style="border-radius:10px;margin:5px;box-shadow:0 0 5px rgba(0,0,0,0.2);">`,
				img,
			)
		}
	} else {
		screensHTML = `<p>No screenshots available</p>`
	}

	html := fmt.Sprintf(`
		<h2>Play Store App Info</h2>
		<div style="display:flex;align-items:center;gap:15px;margin-bottom:10px;">
			<img src="%s" alt="App Icon" width="96" height="96" style="border-radius:16px;box-shadow:0 0 6px rgba(0,0,0,0.2);">
			<h3 style="margin:0;">%s</h3>
		</div>
		<pre>
App Name/ID: %s
Developer: %s
Developer Email: %s
Developer Website: %s
Category: %s
Rating: %s
Total Ratings: %s
Installs: %s
Free: %t
Ad Supported: %t
In-App Purchases: %t
Last Updated: %s
Current Version: %s
Compatibility: %s
Short Description: %s
Full Description: %s
		</pre>
		<h3>Screenshots:</h3>
		<div>%s</div>
		<br><a href="/">⬅ Go Back</a>
	`, app.Icon, app.Title, app.AppName, app.Developer, app.DeveloperEmail, app.DeveloperWebsite,
		app.Category, rating, ratingCount, app.Installs, app.Free, app.AdSupported, app.InAppPurchase,
		app.LastUpdated, app.CurrentVersion, app.Compatibility, app.ShortDesc, app.Description, screensHTML)

	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(html))
}
