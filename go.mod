module ledger

go 1.24.1

toolchain go1.24.4

require (
	github.com/ledongthuc/pdf v0.0.0-20250511090121-5959a4027728
	github.com/lxn/walk v0.0.0-20210112085537-c389da54e794
)

require (
	github.com/lxn/win v0.0.0-20210218163916-a377121e959e // indirect
	golang.org/x/sys v0.0.0-20201018230417-eeed37f84f13 // indirect
	gopkg.in/Knetic/govaluate.v3 v3.0.0-00010101000000-000000000000 // indirect
)

replace golang.org/x/sys => github.com/golang/sys v0.0.0-20201018230417-eeed37f84f13

replace gopkg.in/Knetic/govaluate.v3 => github.com/Knetic/govaluate v3.0.0+incompatible
