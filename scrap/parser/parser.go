// file: playstore_scraper.go
package parser

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/chromedp/chromedp"
)

// App holds extracted Play Store fields (JSON tags for easy marshaling)
type App struct {
	AppName          string   `json:"appName"`
	Title            string   `json:"title"`
	Icon             string   `json:"icon"`
	Developer        string   `json:"developer"`
	DeveloperEmail   string   `json:"developerEmail"`
	DeveloperWebsite string   `json:"developerWebsite"`
	Category         string   `json:"genre"`
	Rating           string   `json:"rating"`
	RatingCount      string   `json:"ratingCount"`
	Installs         string   `json:"installs"`
	Free             bool     `json:"free"`
	AdSupported      bool     `json:"adSupported"`
	InAppPurchase    bool     `json:"InAppPurchase"`
	LastUpdated      string   `json:"updated"`
	CurrentVersion   string   `json:"version"`
	Compatibility    string   `json:"androidVersion"`
	ShortDesc        string   `json:"summary"`
	Description      string   `json:"description"`
	Screenshots      []string `json:"screenshots"`
}

// ScrapePlayStoreByURL scrapes the play store page for the given full URL (recommended)
func ScrapePlayStoreByURL(url string) (*App, error) {
	// create chrome context
	ctx, cancel := chromedp.NewContext(context.Background())
	defer cancel()

	// overall timeout
	ctx, cancel = context.WithTimeout(ctx, 40*time.Second)
	defer cancel()

	// convenience JS helpers:
	// 1) try JSON-LD SoftwareApplication block (returns JSON string or empty)
	jsGetJSONLD := `(function(){
		try{
			const scripts = document.querySelectorAll('script[type="application/ld+json"]');
			for(const s of scripts){
				try{
					const j = JSON.parse(s.textContent);
					// Some pages put an array
					if(j && (j['@type']=='SoftwareApplication' || (Array.isArray(j) && j[0] && j[0]['@type']=='SoftwareApplication'))) {
						return JSON.stringify(j);
					}
				}catch(e){}
			}
		}catch(e){}
		return '';
	})()`

	// 2) generic find that searches light DOM + shadow roots for a selector and returns textContent
	jsFindText := `(function(sel){
		function findIn(root){
			try{
				const el = root.querySelector(sel);
				if(el) return el.textContent.trim();
			}catch(e){}
			// search children for shadow roots
			const nodes = root.querySelectorAll('*');
			for(let i=0;i<nodes.length;i++){
				try{
					if(nodes[i].shadowRoot){
						const r = findIn(nodes[i].shadowRoot);
						if(r) return r;
					}
				}catch(e){}
			}
			return '';
		}
		// try document then shadow tree
		return findIn(document);
	})`

	// 3) generic find attribute (e.g., aria-label)
	jsFindAttr := `(function(sel, attr){
		function findIn(root){
			try{
				const el = root.querySelector(sel);
				if(el) {
					const v = el.getAttribute(attr);
					return v?String(v).trim():'';
				}
			}catch(e){}
			const nodes = root.querySelectorAll('*');
			for(let i=0;i<nodes.length;i++){
				try{
					if(nodes[i].shadowRoot){
						const r = findIn(nodes[i].shadowRoot);
						if(r) return r;
					}
				}catch(e){}
			}
			return '';
		}
		return findIn(document);
	})`

	// 4) gather screenshot urls (collect img srcs that look like play-lh)
	jsCollectScreens := `(function(){
		const out = [];
		function visit(root){
			try{
				const imgs = root.querySelectorAll('img');
				for(const img of imgs){
					const s = (img.src || img.getAttribute('data-src') || img.getAttribute('srcset')||'').toString();
					if(s && (s.includes('play-lh') || s.includes('play.googleusercontent.com'))) {
						if(!out.includes(s)) out.push(s);
					}
				}
			}catch(e){}
			const nodes = root.querySelectorAll('*');
			for(let i=0;i<nodes.length;i++){
				try{ if(nodes[i].shadowRoot) visit(nodes[i].shadowRoot); }catch(e){}
			}
		}
		visit(document);
		return out;
	})()`

	// NEW: robust detail table finder that targets the Play Store details block (jsname="ihy2Pb")

	// run navigation and wait for key elements
	var jsonldRaw string
	var title string

	tasks := chromedp.Tasks{
		chromedp.Navigate(url),

		// Wait a few visible things — safe wait for title and rating area
		chromedp.WaitVisible(`h1 span`, chromedp.ByQuery),
		// rating block may be in shadow DOM; wait an extra bit
		chromedp.Sleep(1200 * time.Millisecond),

		// try to get JSON-LD (fast path)
		chromedp.Evaluate(jsGetJSONLD, &jsonldRaw),

		// also fetch title quickly
		chromedp.Evaluate(jsFindText+`('h1 span')`, &title),
	}

	if err := chromedp.Run(ctx, tasks); err != nil {
		return nil, fmt.Errorf("navigation/render failed: %v", err)
	}

	app := &App{}

	// First, try JSON-LD parse if available
	if jsonldRaw != "" {
		// sometimes page returns an array or object; try to unmarshal into map or []interface{}
		var obj interface{}
		if err := json.Unmarshal([]byte(jsonldRaw), &obj); err == nil {
			switch v := obj.(type) {
			case map[string]interface{}:
				populateFromJSONLDMap(v, app)
			case []interface{}:
				// first element that looks like SoftwareApplication
				for _, el := range v {
					if m, ok := el.(map[string]interface{}); ok {
						if t, _ := m["@type"].(string); strings.Contains(strings.ToLower(t), "softwareapplication") {
							populateFromJSONLDMap(m, app)
							break
						}
					}
				}
			}
		}
	}

	// Helper: evaluate many selectors via shadow-aware JS
	var tmp string
	var screenshots []string

	// Title fallback
	if app.Title == "" {
		if title != "" {
			app.Title = strings.TrimSpace(title)
		} else {
			_ = chromedp.Run(ctx, chromedp.Evaluate(jsFindText+`('h1 span')`, &tmp))
			if tmp != "" {
				app.Title = strings.TrimSpace(tmp)
			}
		}
	}

	// Icon (try meta og:image then simple selector)
	_ = chromedp.Run(ctx, chromedp.Evaluate(jsFindAttr+`('meta[property="og:image"]','content')`, &tmp))
	if tmp != "" {
		app.Icon = tmp
	} else {
		_ = chromedp.Run(ctx, chromedp.Evaluate(jsFindAttr+`('img[itemprop="image"]','src')`, &tmp))
		if tmp != "" {
			app.Icon = tmp
		}
	}

	// Rating (aria-label)
	_ = chromedp.Run(ctx, chromedp.Evaluate(jsFindAttr+`('div[aria-label*="Rated"], div[aria-label*="stars"], div[aria-label*="star"]','aria-label')`, &tmp))
	if tmp != "" {
		// Example: "Rated 4.3 stars out of five stars"
		parts := strings.Fields(tmp)
		if len(parts) > 1 {
			app.Rating = parts[1]
		} else {
			app.Rating = tmp
		}
	}

	// Rating count: try common visible span text or buttons
	_ = chromedp.Run(ctx, chromedp.Evaluate(jsFindText+`('button[aria-label*="ratings"], span[class*="g1rdde"], span[class*="EymY4b"]')`, &tmp))
	if tmp == "" {
		// more aggressive: look for "* ratings" text anywhere (shadow aware)
		_ = chromedp.Run(ctx, chromedp.Evaluate(`(function(){ try{ return document.body.innerText.match(/[\d\.,]+\+?\s*(?:ratings|reviews|ratings)/i)? document.body.innerText.match(/[\d\.,]+\+?\s*(?:ratings|reviews|ratings)/i)[0] : '' }catch(e){return ''}})()`, &tmp))
	}
	if tmp != "" {
		app.RatingCount = strings.TrimSpace(tmp)
	}

	var installs string

	chromedp.Run(ctx, chromedp.Evaluate(`(function(){
    // New Play Store (Dec 2025)
    let el = document.querySelector('div[data-testid="info-section"] div[data-testid="info-section-item"] span:nth-child(2)');
    if (el && el.innerText.trim().match(/[\d\.,]+.*\+/)) return el.innerText.trim();

    // Search ANY element containing "Downloads" label
    let els = document.querySelectorAll('*');
    for (const e of els){
        try {
            let t = (e.innerText||"").toLowerCase();
            if (t.includes("downloads") || t.includes("installs")){
                let num = t.match(/[\d\.,]+.*\+/);
                if(num) return num[0];
            }
        }catch(e){}
    }

    // Shadow DOM scan
    function scanShadow(node){
        try{
            if(node.shadowRoot){
                const root = node.shadowRoot;
                const txt = root.innerText.toLowerCase();
                let m = txt.match(/[\d\.,]+.*\+/);
                if(m) return m[0];
                for(const c of root.querySelectorAll('*')){
                    let r = scanShadow(c);
                    if(r) return r;
                }
            }
        }catch(e){}
        return "";
    }
    for(const n of document.querySelectorAll('*')){
        let r = scanShadow(n);
        if(r) return r;
    }

    return "";
})()`, &installs))

	if installs != "" {
		app.Installs = installs
	} else {
		app.Installs = "N.A"
	}

	var updated string

	chromedp.Run(ctx, chromedp.Evaluate(`(function(){
    //New Play Store 2025 layout
    try {
        let el = document.querySelector('[data-testid="info-section"] div[data-testid="info-section-item"] span:nth-child(2)');
        if (el && el.innerText.toLowerCase().includes("updated")) {
            return el.innerText.replace(/updated/i, "").trim();
        }
    } catch(e){}

    //Search ANY element containing "Updated" text
    let all = document.querySelectorAll("*");
    for (const e of all) {
        try {
            let txt = (e.innerText || "").toLowerCase();
            if (txt.includes("updated on") || txt.includes("updated") || txt.includes("last updated")) {
                let m = txt.match(/updated(?: on)?[: ]*\s*(.*)/i);
                if (m && m[1]) return m[1].trim();
            }
        } catch(e){}
    }

    //Shadow DOM deep scan
    function deep(node){
        try {
            if (node.shadowRoot){
                const t = (node.shadowRoot.innerText || "").toLowerCase();
                let m = t.match(/updated(?: on)?[: ]*\s*(.*)/i);
                if (m && m[1]) return m[1].trim();

                for(const ch of node.shadowRoot.querySelectorAll("*")){
                    let r = deep(ch);
                    if (r) return r;
                }
            }
        }catch(e){}
        return "";
    }

    for(const n of document.querySelectorAll("*")){
        let r = deep(n);
        if (r) return r;
    }

    return "";
})()`, &updated))

	if updated != "" {
		app.LastUpdated = updated
	} else {
		app.LastUpdated = "N.A"
	}

	var currentVersion string

	chromedp.Run(ctx, chromedp.Evaluate(`(function(){

    //New Play Store (2025 UI)
    try {
        let el = document.querySelector('div[data-testid="info-section"] div[data-testid="info-section-item"]');
        if (el) {
            let label = el.innerText.toLowerCase();
            if (label.includes("version")) {
                let parts = el.innerText.split("\n");
                if (parts.length > 1) return parts[1].trim();
            }
        }
    } catch(e){}

    //Old UI lookup: ANY element containing 'current version'
    try {
        let all = document.querySelectorAll('*');
        for (const node of all){
            let txt = (node.innerText||"").toLowerCase();
            if (txt.includes("current version")){
                let match = (node.innerText||"").split("\n");
                if (match.length > 1) return match[1].trim();
                let m2 = node.innerText.match(/[0-9A-Za-z\.\-\_]+/);
                if (m2) return m2[0];
            }
        }
    } catch(e){}

    //Regex scan for version-like patterns: 2.3.1, 1.0.9, v4.7.2 etc
    try {
        let body = document.body.innerText;
        let regex = /\b(v?[\d]+\.[\d]+(?:\.[\d]+)?)\b/g;
        let m = body.match(regex);
        if (m && m.length) return m[0];
    } catch(e){}

    //Shadow DOM deep scan
    function scanShadow(node){
        try {
            if(node.shadowRoot){
                let txt = node.shadowRoot.innerText.toLowerCase();
                if (txt.includes("current version")){
                    let lines = node.shadowRoot.innerText.split("\n");
                    if (lines.length > 1) return lines[1].trim();
                }
                for(const c of node.shadowRoot.querySelectorAll('*')){
                    let r = scanShadow(c);
                    if(r) return r;
                }
            }
        } catch(e){}
        return "";
    }

    for(const n of document.querySelectorAll('*')){
        let r = scanShadow(n);
        if(r) return r;
    }

    return "";

})()`, &currentVersion))

	if currentVersion != "" {
		app.CurrentVersion = currentVersion
	} else {
		app.CurrentVersion = "N.A"
	}

	var compatibility string

	chromedp.Run(ctx, chromedp.Evaluate(`(function(){

    // Extract compatibility from JSON-LD metadata
    try {
        let json = document.querySelector('script[type="application/ld+json"]');
        if (json) {
            let data = JSON.parse(json.innerText);
            if (data.operatingSystem) {
                return data.operatingSystem.trim();
            }
        }
    } catch(e){}

    return ""; // fallback

})()`, &compatibility))

	if compatibility != "" {
		app.Compatibility = compatibility
	} else {
		app.Compatibility = "N.A"
	}

	// Additional installs fallback (regex search)
	if app.Installs == "" {
		_ = chromedp.Run(ctx, chromedp.Evaluate(`(function(){ try{ const m = document.body.innerText.match(/[\d\.,]+\+?\s*(?:downloads|installs|downloads)/i); return m?m[0]:'' }catch(e){return ''}})()`, &tmp))
		if tmp != "" {
			app.Installs = tmp
		}
	}

	// Developer name
	_ = chromedp.Run(ctx, chromedp.Evaluate(jsFindText+`('a.hrTbp.R8zArc, a[href^="/store/apps/dev"]')`, &tmp))
	if tmp != "" {
		app.Developer = tmp
	}

	// Developer email (mailto)
	_ = chromedp.Run(ctx, chromedp.Evaluate(`(function(){ try{ const a = document.querySelector('a[href^="mailto:"]'); if(a) return a.getAttribute('href'); return ''; }catch(e){return ''}})()`, &tmp))
	if tmp != "" {
		app.DeveloperEmail = strings.TrimSpace(tmp)
	}

	// Developer website: look for link under developer block or first external link not play.google
	_ = chromedp.Run(ctx, chromedp.Evaluate(`(function(){ try{
		const links = document.querySelectorAll('a[href^="http"]');
		for(const a of links){
			try{
				const h = a.href;
				if(h && !h.includes('play.google.com') && !h.includes('accounts.google.com')) return h;
			}catch(e){}
		}
		return '';
	}catch(e){return ''}})()`, &tmp))
	if tmp != "" {
		app.DeveloperWebsite = tmp
	}

	// Short summary
	if app.Description != "" {
		desc := app.Description

		// split by sentence delimiters
		parts := strings.Split(desc, ".")

		short := ""
		if len(parts) > 0 {
			short += strings.TrimSpace(parts[0]) + "."
		}
		if len(parts) > 1 {
			short += " " + strings.TrimSpace(parts[1]) + "."
		}

		// Trim & limit max 250 chars (optional)
		short = strings.TrimSpace(short)
		if len(short) > 250 {
			short = short[:250]
		}

		app.ShortDesc = short
	} else {
		app.ShortDesc = "N.A"
	}

	// Full description
	_ = chromedp.Run(ctx, chromedp.Evaluate(`(function(){
    function findDesc(root) {
        try {
            // MAIN FULL DESCRIPTION BLOCK
            const el = root.querySelector('div[jsname="sngebd"], div.DWPxHb, div[data-g-id="description"]');
            if (el && el.innerText.trim()) return el.innerText.trim();
        } catch(e){}
        
        // search through shadow DOM children
        const nodes = root.querySelectorAll("*");
        for (let i = 0; i < nodes.length; i++) {
            try {
                if (nodes[i].shadowRoot) {
                    const r = findDesc(nodes[i].shadowRoot);
                    if (r) return r;
                }
            } catch(e){}
        }
        return "";
    }
    return findDesc(document);
})()`, &tmp))

	if tmp != "" {
		app.Description = strings.TrimSpace(strings.Join(strings.Fields(tmp), " "))
	}

	// Ads / IAP detection (page text search)
	_ = chromedp.Run(ctx, chromedp.Evaluate(`document.body.innerText`, &tmp))
	if tmp != "" {
		lc := strings.ToLower(tmp)
		if strings.Contains(lc, "contains ads") || strings.Contains(lc, "contains advertising") {
			app.AdSupported = true
		}
		if strings.Contains(lc, "in-app purchases") || strings.Contains(lc, "in-app billing") {
			app.InAppPurchase = true
		}
		if strings.Contains(lc, "free") && !strings.Contains(lc, "paid") {
			app.Free = true
		}
	}

	// Category (try JSON-LD or fallback to link text)
	if app.Category == "" {
		_ = chromedp.Run(ctx, chromedp.Evaluate(jsFindText+`('a[itemprop="genre"], a[href*="category"]')`, &tmp))
		if tmp != "" {
			app.Category = tmp
		}
	}

	// Screenshots
	_ = chromedp.Run(ctx, chromedp.Evaluate(jsCollectScreens, &screenshots))

	pick := []int{0, 2, 3, 4, 5}
	selected := []string{}

	for _, idx := range pick {
		if idx < len(screenshots) {
			selected = append(selected, screenshots[idx])
		}
	}

	app.Screenshots = selected

	// final normalization defaults
	if app.CurrentVersion == "" {
		app.CurrentVersion = "N.A"
	}
	if app.Compatibility == "" {
		app.Compatibility = "N.A"
	}
	if app.Installs == "" {
		app.Installs = "N.A"
	}
	if app.Title == "" {
		return nil, fmt.Errorf("app not found or selectors changed")
	}
	// if AppName not present, try extracting id from URL
	if app.AppName == "" {
		// try to parse ?id=... from location
		_ = chromedp.Run(ctx, chromedp.Evaluate(`(function(){ try{ return location.href }catch(e){return ''}})()`, &tmp))
		if u := tmp; strings.Contains(u, "id=") {
			parts := strings.Split(u, "id=")
			if len(parts) > 1 {
				app.AppName = strings.Split(parts[1], "&")[0]
			}
		}
	}

	return app, nil
}

// small helper: populate app from JSON-LD map
func populateFromJSONLDMap(m map[string]interface{}, app *App) {
	if v, ok := m["name"].(string); ok && app.Title == "" {
		app.Title = v
	}
	if v, ok := m["image"].(string); ok && app.Icon == "" {
		app.Icon = v
	}
	if v, ok := m["url"].(string); ok && app.AppName == "" {
		app.AppName = v
	}
	if v, ok := m["description"].(string); ok && app.Description == "" {
		app.Description = v
	}
	if v, ok := m["applicationCategory"].(string); ok && app.Category == "" {
		app.Category = v
	}
	if author, ok := m["author"].(map[string]interface{}); ok {
		if v, ok2 := author["name"].(string); ok2 && app.Developer == "" {
			app.Developer = v
		}
	}
	if agg, ok := m["aggregateRating"].(map[string]interface{}); ok {
		if rv, ok2 := agg["ratingValue"]; ok2 && app.Rating == "" {
			app.Rating = fmt.Sprint(rv)
		}
		if rc, ok2 := agg["ratingCount"]; ok2 && app.RatingCount == "" {
			app.RatingCount = fmt.Sprint(rc)
		}
	}
}
