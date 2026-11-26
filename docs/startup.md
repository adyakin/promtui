# Choose a startup pane

PromTUI opens in PromQL by default. To open a different pane on launch, use
`-alerts`, `-silences`, `-targets`, `-routing`, or `-rules`. Use `-promql` to
select the default explicitly. Choose at most one pane flag.

Pane flags work alongside `-config` and `-connection`. For example:

```sh
promtui -alerts -config config.yaml -connection production
```

Run `promtui -help` for the full list of command-line flags.
