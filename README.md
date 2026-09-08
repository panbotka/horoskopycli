# Horoskopy.cz CLI

[![Go](https://github.com/kozaktomas/horoskopycli/actions/workflows/go.yaml/badge.svg)](https://github.com/kozaktomas/horoskopycli/actions/workflows/go.yaml)

Read your [horoskopy.cz](https://www.horoskopy.cz) horoscope without leaving the terminal.

# How to install

### From source code:

```bash
go install github.com/kozaktomas/horoskopycli@latest
```

### From releases

Download the newest version from [Releases](https://github.com/kozaktomas/horoskopycli/releases) and put the binary to
your $PATH.

# Usage

```bash
horoskopycli <sign> [period]
```

The period is optional and defaults to `dnes`.

```bash
horoskopycli byk          # today
horoskopycli ryby zitra   # tomorrow
horoskopycli lev mesic    # this month
horoskopycli stir rok     # this year
```

**Signs:** `beran`, `byk`, `blizenci`, `rak`, `lev`, `panna`, `vahy`, `stir`, `strelec`, `kozoroh`, `vodnar`, `ryby`

**Periods:** `dnes`, `zitra`, `mesic`, `rok`

Czech diacritics and capitalisation are accepted too, so `horoskopycli Býk zítra` works just as well.

Running `horoskopycli` with no arguments prints this list.

### Example output

```
Ryby zítra
==========

Blízkost se často buduje prostřednictvím obyčejných okamžiků a pozornosti. ...

=> Láska a přátelství
Malé gesto vám připomene, proč jste si právě tohoto člověka pustili k sobě. ...

=> Peníze a práce
Malý pracovní přešlap vás zaskočí víc, než by měl. ...

https://www.horoskopy.cz/clanek/horoskop-znameni-zverokruhu-ryby-zitra-558
```

### Development

Make changes and run `make` and make it pass.

See [docs/architecture.md](docs/architecture.md) for how the tool talks to horoskopy.cz — worth reading before
changing anything in `internal/horoskopy`.

###### Run tests
```bash
make test
```

###### Fix lint
```bash
make lint-fix
```
