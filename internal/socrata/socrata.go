// Package socrata downloads Socrata (SODA) datasets as paged CSV files.
// Pages are written atomically and skipped on re-run, so downloads resume.
package socrata

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

type Dataset struct {
	Name   string // local table/dir name
	ID     string // socrata 4x4 id
	Where  string // SoQL $where clause ("" for all rows)
	Select string // optional $select
}

type Client struct {
	Domain   string // e.g. datacatalog.cookcountyil.gov
	AppToken string // optional; raises rate limits
	PageSize int
	HTTP     *http.Client
	Log      func(format string, args ...any)
}

func (c *Client) pageURL(d Dataset, offset int) string {
	q := url.Values{}
	q.Set("$limit", strconv.Itoa(c.PageSize))
	q.Set("$offset", strconv.Itoa(offset))
	q.Set("$order", ":id")
	if d.Where != "" {
		q.Set("$where", d.Where)
	}
	if d.Select != "" {
		q.Set("$select", d.Select)
	}
	return fmt.Sprintf("https://%s/resource/%s.csv?%s", c.Domain, d.ID, q.Encode())
}

// Fetch downloads all pages of d into dir/page-NNNNN.csv. Returns page count.
func (c *Client) Fetch(ctx context.Context, d Dataset, dir string) (int, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return 0, err
	}
	for page := 0; ; page++ {
		path := filepath.Join(dir, fmt.Sprintf("page-%05d.csv", page))
		var rows int
		if fi, err := os.Stat(path); err == nil && fi.Size() > 0 {
			n, err := countRows(path)
			if err != nil {
				return page, err
			}
			rows = n
		} else {
			n, err := c.fetchPage(ctx, d, page*c.PageSize, path)
			if err != nil {
				return page, fmt.Errorf("%s page %d: %w", d.Name, page, err)
			}
			rows = n
			c.Log("%s: page %d (%d rows)", d.Name, page, rows)
		}
		if rows < c.PageSize {
			// An empty last page is dropped, but an empty first page is kept:
			// its header lets load create an empty table, so a small
			// -pin-prefix build with no condos (say) still loads and builds.
			if rows == 0 && (page > 0 || emptyFile(path)) {
				os.Remove(path)
				return page, nil
			}
			return page + 1, nil
		}
	}
}

// emptyFile reports whether path is missing or has no bytes (no header).
func emptyFile(path string) bool {
	fi, err := os.Stat(path)
	return err != nil || fi.Size() == 0
}

func (c *Client) fetchPage(ctx context.Context, d Dataset, offset int, path string) (int, error) {
	var lastErr error
	for attempt := 0; attempt < 6; attempt++ {
		if attempt > 0 {
			wait := time.Duration(1<<attempt) * time.Second
			c.Log("%s: retry %d in %s (%v)", d.Name, attempt, wait, lastErr)
			select {
			case <-time.After(wait):
			case <-ctx.Done():
				return 0, ctx.Err()
			}
		}
		n, err := c.tryPage(ctx, d, offset, path)
		if err == nil {
			return n, nil
		}
		lastErr = err
	}
	return 0, lastErr
}

func (c *Client) tryPage(ctx context.Context, d Dataset, offset int, path string) (int, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", c.pageURL(d, offset), nil)
	if err != nil {
		return 0, err
	}
	if c.AppToken != "" {
		req.Header.Set("X-App-Token", c.AppToken)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 500))
		return 0, fmt.Errorf("HTTP %d: %s", resp.StatusCode, b)
	}
	tmp := path + ".part"
	f, err := os.Create(tmp)
	if err != nil {
		return 0, err
	}
	if _, err := io.Copy(f, resp.Body); err != nil {
		f.Close()
		os.Remove(tmp)
		return 0, err
	}
	if err := f.Close(); err != nil {
		return 0, err
	}
	n, err := countRows(tmp)
	if err != nil {
		return 0, err
	}
	return n, os.Rename(tmp, path)
}

// countRows counts CSV records (minus header) using a real CSV reader,
// since quoted fields may contain newlines.
func countRows(path string) (int, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	r := newCSV(bufio.NewReaderSize(f, 1<<20))
	n := -1
	for {
		_, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return 0, err
		}
		n++
	}
	if n < 0 {
		n = 0
	}
	return n, nil
}
