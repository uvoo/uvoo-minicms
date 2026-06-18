.PHONY: dev build license-check package package-linux release run web web-audit web-audit-all web-install docker-up docker-build docker-smoke docker-down

web:
	cd web && npm ci && npm run build

web-install:
	cd web && npm ci

web-audit:
	cd web && npm audit --omit=dev --audit-level=high

web-audit-all:
	cd web && npm audit --audit-level=high

build:
	bash scripts/build.sh

license-check:
	bash scripts/license-check.sh

package:
	bash scripts/package.sh

package-linux:
	bash scripts/package-linux.sh

release: web-audit
	bash scripts/release.sh

run: build
	./bin/uvoo-minicms

dev:
	mkdir -p data/uploads
	test -d web/node_modules || $(MAKE) web-install
	( cd web && npm run dev ) & go run ./cmd/uvoo-minicms

docker-build:
	docker compose build uvoo-minicms

docker-smoke:
	bash scripts/docker-smoke.sh

docker-up:
	docker compose up -d --build --remove-orphans uvoo-minicms

docker-down:
	docker compose down
