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

func (v *XPathDBRouter) RegisterRoute(db TorrServerDB, xPath string) error {
	newRoute := v.xPathToRoute(xPath)

	if slices.Contains(v.routes, newRoute) {
		return fmt.Errorf("route \"%s\" already in routing table", newRoute)
	}

	// First DB becomes Default DB with default route
	if len(v.dbs) == 0 && len(newRoute) != 0 {
		if err := v.RegisterRoute(db, ""); err != nil {
			return err
		}
	}

	if !slices.Contains(v.dbs, db) {
		v.dbs = append(v.dbs, db)
		v.dbNames[db] = reflect.TypeOf(db).Elem().Name()
		v.log(fmt.Sprintf("Registered new DB \"%s\", total %d DBs registered", v.getDBName(db), len(v.dbs)))
	}

	v.route2db[newRoute] = db
	v.routes = append(v.routes, newRoute)

	// Sort routes by length descending.
	//   It is important later to help selecting
	//   most suitable route in getDBForXPath(xPath)
	sort.Slice(v.routes, func(iLeft, iRight int) bool {
		return len(v.routes[iLeft]) > len(v.routes[iRight])
	})
	v.log(fmt.Sprintf("Registered new route \"%s\" for DB \"%s\", total %d routes", getDefaultRoureName(newRoute), v.getDBName(db), len(v.routes)))

	return nil
}

func getDefaultRoureName(route string) string {
	if len(route) > 0 {
		return route
	}

	return "default"
}

func (v *XPathDBRouter) xPathToRoute(xPath string) string {
	return strings.ToLower(strings.TrimSpace(xPath))
}

func (v *XPathDBRouter) getDBForXPath(xPath string) TorrServerDB {
	if len(v.dbs) == 0 {
		return nil
	}

	lookupRoute := v.xPathToRoute(xPath)

	var db TorrServerDB = nil

	for _, routePrefix := range v.routes {
		if strings.HasPrefix(lookupRoute, routePrefix) {
			db = v.route2db[routePrefix]

			break
		}
	}

	return db
}

func (v *XPathDBRouter) Get(xPath, name string) []byte {
	if v == nil {
		return nil
	}

	db := v.getDBForXPath(xPath)
	if db == nil {
		return nil
	}

	return db.Get(xPath, name)
}

func (v *XPathDBRouter) Set(xPath, name string, value []byte) {
	if v == nil {
		return
	}

	db := v.getDBForXPath(xPath)
	if db == nil {
		return
	}

	db.Set(xPath, name, value)
}

func (v *XPathDBRouter) List(xPath string) []string {
	if v == nil {
		return nil
	}

	db := v.getDBForXPath(xPath)
	if db == nil {
		return nil
	}

	return db.List(xPath)
}

func (v *XPathDBRouter) Rem(xPath, name string) {
	if v == nil {
		return
	}

	db := v.getDBForXPath(xPath)
	if db == nil {
		return
	}

	db.Rem(xPath, name)
}

func (v *XPathDBRouter) Clear(xPath string) {
	if v == nil {
		return
	}

	db := v.getDBForXPath(xPath)
	if db == nil {
		return
	}

	db.Clear(xPath)
}

func (v *XPathDBRouter) CloseDB() {
	for _, db := range v.dbs {
		db.CloseDB()
	}

	v.dbs = nil
	v.routes = nil
	v.route2db = nil
	v.dbNames = nil
}

func (v *XPathDBRouter) getDBName(db TorrServerDB) string {
	return v.dbNames[db]
}

func (v *XPathDBRouter) log(s string, params ...any) {
	if len(params) > 0 {
		log.TLogln(fmt.Sprintf("XPathDBRouter: %s: %s", s, fmt.Sprint(params...)))
	} else {
		log.TLogln("XPathDBRouter: " + s)
	}
}
