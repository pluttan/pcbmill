VENV := venv
PY   := $(VENV)/bin/python3.12
PIP  := $(VENV)/bin/pip

GERBERS ?=
TAP     ?=
LOG     ?=
DIALECT ?= grbl
SIDE    ?= auto

.PHONY: all install cam probe level show test clean

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

test:             ## сквозная проверка на примерах из examples/gerbers
	$(PY) tests/check.py examples/gerbers/*/

clean:
	rm -rf $(VENV) __pycache__ pcbmill/__pycache__ *.pyc
