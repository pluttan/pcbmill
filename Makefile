TAP ?= examples/Vesi_v4.tap
DIALECT ?= grbl

.PHONY: probe level show

probe:            ## программа щупа по программе TAP
	python3 pcbprobe.py grid $(TAP) --dialect $(DIALECT) -o probe.nc

level:            ## LOG=лог касаний → карта → TAP_level
	python3 pcbprobe.py map $(LOG) --tap $(TAP) -o heightmap.csv
	python3 pcbprobe.py apply $(TAP) heightmap.csv --strip-m6

show:
	python3 pcbprobe.py show heightmap.csv
