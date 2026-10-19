APP_NAME := PicPress
EXEC := picpress
BIN := bin/$(EXEC)
APP := bin/$(APP_NAME).app
GO_TAGS := production,nodynamic
FRONTEND := frontend
VERSION := $(shell cat VERSION 2>/dev/null || echo 0.0.0)

.PHONY: build universal app app-universal bundle run dev frontend-install frontend-build test check clean

build: frontend-build
	GOOS=darwin CGO_ENABLED=1 go build -tags "$(GO_TAGS)" -trimpath -buildvcs=false -ldflags="-s -w" -o "$(BIN)" .

universal: frontend-build
	GOOS=darwin GOARCH=arm64 CGO_ENABLED=1 go build -tags "$(GO_TAGS)" -trimpath -buildvcs=false -ldflags="-s -w" -o bin/picpress-arm64 .
	GOOS=darwin GOARCH=amd64 CGO_ENABLED=1 go build -tags "$(GO_TAGS)" -trimpath -buildvcs=false -ldflags="-s -w" -o bin/picpress-amd64 .
	lipo -create -output "$(BIN)" bin/picpress-arm64 bin/picpress-amd64
	rm -f bin/picpress-arm64 bin/picpress-amd64

app: build
	$(MAKE) bundle

app-universal: universal
	$(MAKE) bundle

bundle:
	rm -rf "$(APP)"
	mkdir -p "$(APP)/Contents/MacOS" "$(APP)/Contents/Resources"
	cp "$(BIN)" "$(APP)/Contents/MacOS/$(EXEC)"
	cp build/darwin/Info.plist "$(APP)/Contents/Info.plist"
	cp build/darwin/icons.icns "$(APP)/Contents/Resources/icons.icns"
	/usr/libexec/PlistBuddy -c "Set :CFBundleShortVersionString $(VERSION)" "$(APP)/Contents/Info.plist"
	/usr/libexec/PlistBuddy -c "Set :CFBundleVersion $(VERSION)" "$(APP)/Contents/Info.plist"
	xattr -cr "$(APP)"
	codesign --force --deep --sign - "$(APP)"
	codesign --verify --deep --strict "$(APP)"
	@echo "Built $(APP) v$(VERSION)"

run: build
	./$(BIN)

dev: frontend-install
	./scripts/dev.sh

frontend-install:
	pnpm --dir $(FRONTEND) install --frozen-lockfile

frontend-build: frontend-install
	rm -rf $(FRONTEND)/dist
	mkdir -p $(FRONTEND)/dist
	touch $(FRONTEND)/dist/.gitkeep
	pnpm --dir $(FRONTEND) build

test: frontend-install
	go test ./...
	pnpm --dir $(FRONTEND) exec vp test run

check: frontend-install
	pnpm --dir $(FRONTEND) check
	go vet ./...

clean:
	rm -rf bin
	rm -rf $(FRONTEND)/dist
	mkdir -p $(FRONTEND)/dist
	touch $(FRONTEND)/dist/.gitkeep
