This is a local copy of `github.com/lxn/walk` v0.0.0-20210112085537-c389da54e794,
retained with its upstream license and source. The only behavior patch disables
automatic tooltip registration in `widget.go`. The upstream `TTM_ADDTOOL` call
failed during native window construction on the tested Windows environment,
preventing all UI from starting. The companion does not use tooltips. Revisit
the dependency or remove this local copy when the upstream failure is resolved.
