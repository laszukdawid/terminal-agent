# Terminal Agent Documentation

This directory contains the documentation for the Terminal Agent project, built with MkDocs.

## Running the Documentation Locally

### Prerequisites

1. Make sure you have Python installed (Python 3.6 or higher is recommended)
2. Install MkDocs and required packages:

```sh
pip install mkdocs-material
```

### Viewing the Documentation

From the root directory of the project, run:

```sh
mkdocs serve
```

This will start a local server at http://127.0.0.1:8000/ where you can preview your documentation.

Alternatively, use the provided script:

```sh
./docs/start-docs.sh
```

### Building the Documentation

To build the static site:

```sh
mkdocs build
```

This will create a `site` directory with the compiled HTML files.

## Documentation Structure

- `mkdocs.yml` - Configuration file for MkDocs (at the project root)
- `docs/` - Directory containing all documentation content
  - `index.md` - Homepage
  - `*.md` - Various documentation pages
  - `commands/` - Command-specific documentation
  - `assets/` - Images, CSS, and other assets

## Adding Documentation

1. Create or edit Markdown (`.md`) files in the `docs` directory
2. Update the `nav` section in `mkdocs.yml` to include your new pages
3. Run `mkdocs serve` to preview changes

## Deployment

Documentation is automatically deployed to GitHub Pages when changes are pushed to the `docs` branch using a GitHub Actions workflow. The workflow:

1. Builds the MkDocs site using the Material theme
2. Deploys the built site to GitHub Pages

You can view the current workflow status in the Actions tab of the GitHub repository.
