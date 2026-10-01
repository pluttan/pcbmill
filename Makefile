VENV := venv
PY   := $(VENV)/bin/python3.12
PIP  := $(VENV)/bin/pip

GERBERS ?=
TAP     ?=
LOG     ?=
DIALECT ?= grbl
SIDE    ?= auto

.PHONY: all install cam probe level show check selftest scan-demo win7 test clean

all: install

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

scan-demo:        ## обход сетки на симуляторе станка, без железа
	$(PY) -m pcbmill.scan grid --machine sim --area 0,0,60,40 -y -o scan-demo.csv

GO ?= go1.20      # last Go that still targets Windows 7

win7:             ## win7/pcbscan.exe for the NcStudio PC (Go 1.20, 32-bit)
	cd win7 && $(GO) test . && GOOS=windows GOARCH=386 CGO_ENABLED=0 $(GO) build -trimpath -ldflags "-s -w" -o pcbscan.exe .

test:             ## опасные программы ловятся, свои проходят, обход на симуляторе, сквозная цепочка
	$(PY) tests/test_verify.py
	$(PY) tests/test_scan.py
	cd win7 && $(GO) test .
	$(PY) tests/check.py examples/gerbers/*/

clean:
	rm -rf $(VENV) __pycache__ pcbmill/__pycache__ *.pyc
