-- The proof that a PostGIS add-on works in zonky's PostgreSQL, run by
-- verify-linux.sh and verify-darwin.sh with psql -X -e -v ON_ERROR_STOP=1 and
-- these variables: postgis, geos, proj (expected versions) and projdb (the
-- proj.db the add-on ships, as the server sees its path). Every check raises
-- an error when it fails, so psql exits non-zero; the results are printed too.
--
-- At the end the backend that has loaded PostGIS is left open while the
-- command in $INSPECT runs (\! below) with its pid in $BACKEND, to list what
-- the process has mapped and open.

CREATE FUNCTION pg_temp.check(what text, ok boolean) RETURNS text LANGUAGE plpgsql AS $$
BEGIN
  IF ok IS NOT TRUE THEN
    RAISE EXCEPTION 'proof failed: %', what;
  END IF;
  RETURN 'ok: ' || what;
END $$;

SELECT version();

CREATE EXTENSION postgis;
SELECT postgis_full_version();
SELECT pg_temp.check('PostGIS ' || :'postgis', postgis_lib_version() = :'postgis');
SELECT pg_temp.check('GEOS ' || :'geos', postgis_geos_version() LIKE :'geos' || '-%');
SELECT pg_temp.check('PROJ ' || :'proj', postgis_proj_version() LIKE :'proj' || ' %');
SELECT pg_temp.check('proj.db is the add-on''s: ' || :'projdb',
                     postgis_proj_version() LIKE '%DATABASE_PATH=' || :'projdb' || '%');

-- GEOS: a bow-tie polygon is not valid.
SELECT ST_IsValid('POLYGON((0 0,1 1,1 0,0 1,0 0))'::geometry) AS bowtie_valid;
SELECT pg_temp.check('bow-tie is invalid', NOT ST_IsValid('POLYGON((0 0,1 1,1 0,0 1,0 0))'::geometry));

-- Geodesy: a 5 km buffer on the sphere is about pi * 5000^2 = 78 539 816 m2
-- (32 segments make it 0.6 % smaller).
SELECT ST_Area(ST_Buffer(ST_SetSRID(ST_MakePoint(18.42, -33.92), 4326)::geography, 5000)) AS buffer_m2;
SELECT pg_temp.check('geography buffer area',
  abs(ST_Area(ST_Buffer(ST_SetSRID(ST_MakePoint(18.42, -33.92), 4326)::geography, 5000)) / (pi() * 5000 ^ 2) - 1) < 0.01);

-- PROJ: by SRID (spatial_ref_sys), and by EPSG codes alone, which only proj.db
-- can resolve.
SELECT ST_AsText(ST_Transform(ST_SetSRID(ST_MakePoint(18.42, -33.92), 4326), 32734)) AS by_srid;
SELECT ST_AsText(ST_Transform(ST_MakePoint(18.42, -33.92), 'EPSG:4326', 'EPSG:32734')) AS by_epsg_strings;
SELECT pg_temp.check('ST_Transform by SRID',
  ST_DWithin(ST_Transform(ST_SetSRID(ST_MakePoint(18.42, -33.92), 4326), 32734),
             ST_SetSRID(ST_MakePoint(261488.826, 6243716.333), 32734), 0.01));
SELECT pg_temp.check('ST_Transform by EPSG strings (proj.db)',
  ST_DWithin(ST_Transform(ST_MakePoint(18.42, -33.92), 'EPSG:4326', 'EPSG:32734'),
             ST_MakePoint(261488.826, 6243716.333), 0.01));

-- GEOS overlay: the hole cut out of a square does not cover a point in the hole.
SELECT ST_Covers(ST_Difference('POLYGON((0 0,10 0,10 10,0 10,0 0))'::geometry,
                               'POLYGON((2 2,4 2,4 4,2 4,2 2))'::geometry),
                 'POINT(3 3)'::geometry) AS covers_hole;
SELECT pg_temp.check('difference does not cover the hole',
  NOT ST_Covers(ST_Difference('POLYGON((0 0,10 0,10 10,0 10,0 0))'::geometry,
                              'POLYGON((2 2,4 2,4 4,2 4,2 2))'::geometry),
                'POINT(3 3)'::geometry));

-- GeoJSON in and out (json-c).
SELECT ST_AsGeoJSON(ST_GeomFromGeoJSON('{"type":"Polygon","coordinates":[[[0,0],[1,0],[1,1],[0,0]]]}')) AS geojson;
SELECT pg_temp.check('GeoJSON round trip',
  ST_AsGeoJSON(ST_GeomFromGeoJSON('{"type":"Point","coordinates":[18.42,-33.92]}')) = '{"type":"Point","coordinates":[18.42,-33.92]}');

-- GML (PostGIS's own libxml2) and core xpath() (the server's libxml2), side by side.
SELECT ST_AsText(ST_GeomFromGML('<gml:Point xmlns:gml="http://www.opengis.net/gml" srsName="EPSG:4326"><gml:coordinates>18.42,-33.92</gml:coordinates></gml:Point>')) AS from_gml;
SELECT xpath('/a/text()', '<a>core xml still works</a>'::xml) AS core_xpath;
SELECT pg_temp.check('GML in', ST_AsText(ST_GeomFromGML('<gml:Point xmlns:gml="http://www.opengis.net/gml"><gml:coordinates>18.42,-33.92</gml:coordinates></gml:Point>')) = 'POINT(18.42 -33.92)');
SELECT pg_temp.check('core xpath()', (xpath('/a/text()', '<a>core xml still works</a>'::xml))[1]::text = 'core xml still works');

-- Vector tiles (protobuf-c).
SELECT length(ST_AsMVT(q)) AS mvt_bytes FROM (SELECT ST_AsMVTGeom('POINT(1 1)'::geometry, ST_MakeEnvelope(0, 0, 4096, 4096)) AS geom) q;
SELECT pg_temp.check('MVT encodes', (SELECT length(ST_AsMVT(q)) > 0 FROM (SELECT ST_AsMVTGeom('POINT(1 1)'::geometry, ST_MakeEnvelope(0, 0, 4096, 4096)) AS geom) q));

-- A GiST index, and the planner using it.
CREATE TABLE sites (id int PRIMARY KEY, geom geometry(Point, 4326));
INSERT INTO sites SELECT i, ST_SetSRID(ST_MakePoint(18 + i / 10000.0, -34 + i / 10000.0), 4326) FROM generate_series(1, 10000) i;
CREATE INDEX sites_geom_gist ON sites USING gist (geom);
ANALYZE sites;
SET enable_seqscan = off;
EXPLAIN (COSTS OFF) SELECT count(*) FROM sites WHERE geom && ST_MakeEnvelope(18.1, -33.9, 18.2, -33.8, 4326);
SELECT count(*) AS in_box FROM sites WHERE geom && ST_MakeEnvelope(18.1, -33.9, 18.2, -33.8, 4326);
CREATE FUNCTION pg_temp.plan_uses(idx text) RETURNS boolean LANGUAGE plpgsql AS $$
DECLARE line text;
BEGIN
  FOR line IN EXECUTE 'EXPLAIN (COSTS OFF) SELECT count(*) FROM sites WHERE geom && ST_MakeEnvelope(18.1, -33.9, 18.2, -33.8, 4326)' LOOP
    IF line LIKE '%Index Scan on ' || idx || '%' THEN RETURN true; END IF;
  END LOOP;
  RETURN false;
END $$;
SELECT pg_temp.check('the GiST index is used', pg_temp.plan_uses('sites_geom_gist'));
SELECT pg_temp.check('the index finds the 1001 points', (SELECT count(*) FROM sites WHERE geom && ST_MakeEnvelope(18.1, -33.9, 18.2, -33.8, 4326)) = 1001);

SELECT pg_backend_pid() AS pid \gset
\setenv BACKEND :pid
\echo == this backend (pid :pid), with PostGIS loaded:
\! sh "$INSPECT"
\echo == proof passed
