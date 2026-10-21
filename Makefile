APP_NAME := PicPress
EXEC := picpress
BIN := bin/$(EXEC)
APP := bin/$(APP_NAME).app
GO_TAGS := production,nodynamic
FRONTEND := frontend
VERSION := $(shell cat VERSION 2>/dev/null || echo 0.0.0)
MACOS_MIN := $(shell /usr/libexec/PlistBuddy -c "Print :LSMinimumSystemVersion" build/darwin/Info.plist)
# Bundled ffmpeg / ffprobe for video frame extraction (scripts/build-ffmpeg.sh).
FFMPEG_VERSION := 9.0.2
FFMPEG_SHA256 := 8c3850283eb25fa026482078a04051e0be17347b09ef81a0849bec15a96e002e
FFMPEG_DIR := .bin/ffmpeg-$(FFMPEG_VERSION)
BIN_ARCHS = $(shell lipo -archs "$(BIN)" 2>/dev/null)

.PHONY: build universal app app-universal bundle run dev frontend-install frontend-build test check clean

build: frontend-build
	GOOS=darwin CGO_ENABLED=1 go build -tags "$(GO_TAGS)" -trimpath -buildvcs=false -ldflags="-s -w -X main.appVersion=$(VERSION)" -o "$(BIN)" .

universal: frontend-build
	GOOS=darwin GOARCH=arm64 CGO_ENABLED=1 go build -tags "$(GO_TAGS)" -trimpath -buildvcs=false -ldflags="-s -w -X main.appVersion=$(VERSION)" -o bin/picpress-arm64 .
	GOOS=darwin GOARCH=amd64 CGO_ENABLED=1 go build -tags "$(GO_TAGS)" -trimpath -buildvcs=false -ldflags="-s -w -X main.appVersion=$(VERSION)" -o bin/picpress-amd64 .
	lipo -create -output "$(BIN)" bin/picpress-arm64 bin/picpress-amd64
	rm -f bin/picpress-arm64 bin/picpress-amd64

app: build
	$(MAKE) bundle

app-universal: universal
	$(MAKE) bundle

$(FFMPEG_DIR)/%/ffmpeg $(FFMPEG_DIR)/%/ffprobe: scripts/build-ffmpeg.sh
	FFMPEG_VERSION=$(FFMPEG_VERSION) FFMPEG_SHA256=$(FFMPEG_SHA256) MACOSX_DEPLOYMENT_TARGET=$(MACOS_MIN) \
		./scripts/build-ffmpeg.sh $* "$(FFMPEG_DIR)"

# ffmpeg / ffprobe carry exactly the architectures of $(BIN).
bundle: $(foreach a,$(BIN_ARCHS),$(FFMPEG_DIR)/$(a)/ffmpeg $(FFMPEG_DIR)/$(a)/ffprobe)
	rm -rf "$(APP)"
	mkdir -p "$(APP)/Contents/MacOS" "$(APP)/Contents/Resources"
	cp "$(BIN)" "$(APP)/Contents/MacOS/$(EXEC)"
	cp build/darwin/Info.plist "$(APP)/Contents/Info.plist"
	cp build/darwin/icons.icns "$(APP)/Contents/Resources/icons.icns"
	for t in ffmpeg ffprobe; do \
		lipo -create $(foreach a,$(BIN_ARCHS),"$(FFMPEG_DIR)/$(a)/$$t") -output "$(APP)/Contents/Resources/$$t"; \
	done
	cp "$(FFMPEG_DIR)/COPYING.LGPLv2.1" "$(APP)/Contents/Resources/FFmpeg-COPYING.LGPLv2.1"
	/usr/libexec/PlistBuddy -c "Set :CFBundleShortVersionString $(VERSION)" "$(APP)/Contents/Info.plist"
	/usr/libexec/PlistBuddy -c "Set :CFBundleVersion $(VERSION)" "$(APP)/Contents/Info.plist"
	xattr -cr "$(APP)"
	codesign --force --sign - "$(APP)/Contents/Resources/ffmpeg" "$(APP)/Contents/Resources/ffprobe"
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
