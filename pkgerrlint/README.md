# pkgerrlint

`pkgerrlint` is a linting utility for the Go programming language that analyzes
a Go module and detects instances of `fmt.Errorf` when used with the `%w`
formatting verb. If detected, it suggests using `github.com/pkg/errors.Wrap`
instead of `fmt.Errorf` and can automatically rewrite the code to use the
recommended package.

## Installation

To install `pkgerrlint`, follow these steps:

1. Clone the `azure-container-networking` repository:

```
git clone https://github.com/Azure/azure-container-networking.git
```

2. Navigate to the project directory:

```
cd pkgerrlint
```

3. Build the binary:

```
go build -o pkgerrlint
```

Optionally, you can add the binary to your `$PATH` or move it to a directory
that is already in your `$PATH`.

## Usage

To analyze a Go module, run the following command:

```
./pkgerrlint /path/to/go/module
```

This command will print the file, line, and column of any detected instances of
`fmt.Errorf` with the `%w` verb, along with the message "use
`github.com/pkg/errors.Wrap` to wrap errors instead of `fmt.Errorf`."

To automatically rewrite instances of `fmt.Errorf` with the `%w` verb to use
`errors.Wrap` or `errors.WrapF` instead, run the command with the `--rewrite`
flag:

```
./pkgerrlint --rewrite /path/to/go/module
```

Please note that this utility assumes the `github.com/pkg/errors` package is
imported in the source files. It may be necessary to manually alter the imports
of modified files using a utility like `goimports`.
