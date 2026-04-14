package settings

import (
	"fmt"
	"reflect"
	"sort"
	"strings"

	"server/log"

	"golang.org/x/exp/slices"
)

type XPathDBRouter struct {
	dbs      []TorrServerDB
	routes   []string
	route2db map[string]TorrServerDB
	dbNames  map[TorrServerDB]string
}

func NewXPathDBRouter() *XPathDBRouter {
	router := &XPathDBRouter{
		dbs:      []TorrServerDB{},
		dbNames:  map[TorrServerDB]string{},
		routes:   []string{},
		route2db: map[string]TorrServerDB{},
	}

	return router
}

func (r *XPathDBRouter) GetRawDB() any { return nil }

func (r *XPathDBRouter) RegisterRoute(db TorrServerDB, xPath string) error {
	newRoute := r.xPathToRoute(xPath)

	if slices.Contains(r.routes, newRoute) {
		return fmt.Errorf("route \"%s\" already in routing table", newRoute)
	}

	// First DB becomes Default DB with default route
	if len(r.dbs) == 0 && len(newRoute) != 0 {
		if err := r.RegisterRoute(db, ""); err != nil {
			return err
		}
	}

	if !slices.Contains(r.dbs, db) {
		r.dbs = append(r.dbs, db)
		r.dbNames[db] = reflect.TypeOf(db).Elem().Name()
		r.log(fmt.Sprintf("Registered new DB \"%s\", total %d DBs registered", r.getDBName(db), len(r.dbs)))
	}

	r.route2db[newRoute] = db
	r.routes = append(r.routes, newRoute)

	// Sort routes by length descending.
	//   It is important later to help selecting
	//   most suitable route in getDBForXPath(xPath)
	sort.Slice(r.routes, func(iLeft, iRight int) bool {
		return len(r.routes[iLeft]) > len(r.routes[iRight])
	})
	r.log(fmt.Sprintf("Registered new route \"%s\" for DB \"%s\", total %d routes", getDefaultRoureName(newRoute), r.getDBName(db), len(r.routes)))

	return nil
}

func getDefaultRoureName(route string) string {
	if len(route) > 0 {
		return route
	}

	return "default"
}

func (r *XPathDBRouter) xPathToRoute(xPath string) string {
	return strings.ToLower(strings.TrimSpace(xPath))
}

func (r *XPathDBRouter) getDBForXPath(xPath string) TorrServerDB {
	if len(r.dbs) == 0 {
		return nil
	}

	lookupRoute := r.xPathToRoute(xPath)

	var db TorrServerDB = nil

	for _, routePrefix := range r.routes {
		if strings.HasPrefix(lookupRoute, routePrefix) {
			db = r.route2db[routePrefix]

			break
		}
	}

	return db
}

func (r *XPathDBRouter) Get(xPath, name string) []byte {
	if r == nil {
		return nil
	}

	db := r.getDBForXPath(xPath)
	if db == nil {
		return nil
	}

	return db.Get(xPath, name)
}

func (r *XPathDBRouter) Set(xPath, name string, value []byte) {
	if r == nil {
		return
	}

	db := r.getDBForXPath(xPath)
	if db == nil {
		return
	}

	db.Set(xPath, name, value)
}

func (r *XPathDBRouter) List(xPath string) []string {
	if r == nil {
		return nil
	}

	db := r.getDBForXPath(xPath)
	if db == nil {
		return nil
	}

	return db.List(xPath)
}

func (r *XPathDBRouter) Rem(xPath, name string) {
	if r == nil {
		return
	}

	db := r.getDBForXPath(xPath)
	if db == nil {
		return
	}

	db.Rem(xPath, name)
}

func (r *XPathDBRouter) Clear(xPath string) {
	if r == nil {
		return
	}

	db := r.getDBForXPath(xPath)
	if db == nil {
		return
	}

	db.Clear(xPath)
}

func (r *XPathDBRouter) CloseDB() {
	for _, db := range r.dbs {
		db.CloseDB()
	}

	r.dbs = nil
	r.routes = nil
	r.route2db = nil
	r.dbNames = nil
}

func (r *XPathDBRouter) getDBName(db TorrServerDB) string {
	return r.dbNames[db]
}

func (r *XPathDBRouter) log(s string, params ...any) {
	if len(params) > 0 {
		log.TLogln(fmt.Sprintf("XPathDBRouter: %s: %s", s, fmt.Sprint(params...)))
	} else {
		log.TLogln("XPathDBRouter: " + s)
	}
}
