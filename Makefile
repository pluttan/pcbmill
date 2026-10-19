VENV := venv
PY   := $(VENV)/bin/python3.12
PIP  := $(VENV)/bin/pip

GERBERS ?=
TAP     ?=
LOG     ?=
DIALECT ?= grbl
SIDE    ?= auto

.PHONY: all rules rules-publish testboard install cam probe level show check selftest calib scan-demo win7 mac article-figs test clean

all: install

RULES_HOST ?= root@157.22.189.111
RULES_DIR  ?= /var/www/pcb

rules:            ## правила проектирования плат → docs/pcb-rules.pdf и build/pcb-rules/
	typst compile --font-path docs/fonts docs/pcb-rules.typ docs/pcb-rules.pdf
	mkdir -p build/pcb-rules
	python3 tools/pcb_rules_html.py docs/pcb-rules.json build/pcb-rules
	cp docs/pcb-rules.pdf docs/pcbmill.kicad_dru docs/fonts/GOSTtypeB.ttf build/pcb-rules/

KICAD ?= /Applications/KiCad/KiCad.app/Contents/MacOS/kicad-cli
TB    := build/testboard

testboard:        ## тестовая плата → testboard/pcbmill-test.zip для pcbscan
	rm -rf $(TB) && python3 testboard/gen.py $(TB)
	cp docs/pcbmill.kicad_dru $(TB)/pcbmill-test.kicad_dru
	$(KICAD) pcb drc --refill-zones --save-board --format json -o $(TB)/drc.json $(TB)/pcbmill-test.kicad_pcb >/dev/null
	python3 testboard/drc_summary.py $(TB)/drc.json
	$(KICAD) pcb export gerbers --layers F.Cu,F.Mask,Edge.Cuts -o $(TB)/gerber/ $(TB)/pcbmill-test.kicad_pcb >/dev/null
	$(KICAD) pcb export drill --format excellon --excellon-units mm -o $(TB)/gerber/ $(TB)/pcbmill-test.kicad_pcb >/dev/null
	rm -f testboard/pcbmill-test.zip && cd $(TB)/gerber && zip -qX ../../../testboard/pcbmill-test.zip *
	$(KICAD) pcb export pdf --layers F.Cu,F.Mask,Edge.Cuts --mode-single -o $(TB)/pcbmill-test.pdf $(TB)/pcbmill-test.kicad_pcb >/dev/null

rules-publish: rules  ## страница правил на https://157.22.189.111:8445/
	COPYFILE_DISABLE=1 tar -C build/pcb-rules -cf - . | ssh $(RULES_HOST) 'mkdir -p $(RULES_DIR) && tar -C $(RULES_DIR) --no-same-owner -xf - && chmod -R a+rX $(RULES_DIR)'

install:
	python3.12 -m venv $(VENV) && $(PIP) install -U pip && $(PIP) install -r requirements.txt

cam:              ## GERBERS=папка → GERBERS/cnc/*.tap и превью
	$(PY) -m pcbmill cam $(GERBERS) --side $(SIDE)

probe:            ## TAP=программа → probe.nc
	$(PY) -m pcbmill probe $(TAP) --dialect $(DIALECT) -o probe.nc

level:            ## TAP=программа LOG=лог касаний → heightmap.csv и TAP_level
	$(PY) -m pcbmill map $(LOG) --tap $(TAP) -o heightmap.csv
	$(PY) -m pcbmill apply $(TAP) heightmap.csv

show:
	$(PY) -m pcbmill show heightmap.csv

check:            ## TAP="a.tap b.tap" → проверка перед запуском на станке
	$(PY) -m pcbmill check $(TAP)

selftest:         ## пробные программы для станка → selftest/
	$(PY) -m pcbmill selftest -o selftest

calib:            ## калибровочные платы: подача x обороты, ширины, зазоры → examples/calib/
	$(PY) -m pcbmill calib -o examples/calib

scan-demo:        ## обход сетки на симуляторе станка, без железа
	$(PY) -m pcbmill.scan grid --machine sim --area 0,0,60,40 -y -o scan-demo.csv

GO ?= go1.20      # last Go that still targets Windows 7

win7:             ## win7/pcbscan.exe for the NcStudio PC (Go 1.20, 32-bit, a window without a console)
	cd win7 && $(GO) test ./... && GOOS=windows GOARCH=386 $(GO) vet ./... && \
		GOOS=windows GOARCH=386 CGO_ENABLED=0 $(GO) build -trimpath -ldflags "-s -w -H windowsgui" -o pcbscan.exe .

mac:              ## win7/mac/: pcbscan for macOS (Apple Silicon) with two boards; opens in the browser, emulation is on
	cd win7 && $(GO) test ./... && GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 $(GO) build -trimpath -o mac/pcbscan . && \
		cp testdata/gerbers/2032Charger.zip mac/ && mkdir -p mac/eagle-demo && cp ../examples/gerbers/eagle/* mac/eagle-demo/

article-figs:     ## рисунки статьи → docs/article/img/ (данные и кадры снимают тесты win7/fig*_test.go)
	cd win7 && FIG_OUT=../docs/article/data/star.json FIG_CROP=147.5,-134,156.5,-125 \
		$(GO) test -tags figs -run FigDump .
	cd win7 && FIG_MAP=../docs/article/data/first_map.csv FIG_OUT=../docs/article/img/firstmap.png \
		FIG_SIZE=1440x900 $(GO) test -tags figs -run FigMap .
	rm -rf docs/article/data/shots
	cd win7 && PCBSCAN_SHOTS=../docs/article/data/shots PCBSCAN_SIZE=1440x900 \
		$(GO) test -tags figs -run 'TestFigStages$$' .
	for f in 01_programs 02_sheet 04_scan_running 05_map_result; do \
		cp docs/article/data/shots/$$f.png docs/article/img/app_$${f#*_}.png; done
	$(PY) docs/article/figures.py

test:             ## опасные программы ловятся, свои проходят, обход на симуляторе, сквозная цепочка
	$(PY) tests/test_verify.py
	$(PY) tests/test_scan.py
	$(PY) tests/test_calib.py
	cd win7 && $(GO) test ./...
	$(PY) tests/check.py examples/gerbers/*/

clean:
	rm -rf $(VENV) __pycache__ pcbmill/__pycache__ *.pyc
