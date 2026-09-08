{
  description = "mcli — a command-line interface for monday.com's GraphQL API, built for LLM agents";

  inputs.nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";

  outputs =
    { self, nixpkgs }:
    let
      # Bumped at release time, and checked against the tag by the release
      # workflow before anything is published. Nix builds from a source tree with
      # no .git, so `git describe` is unavailable here and the version has to be
      # stated literally.
      version = "0.8.3";

      systems = [
        "x86_64-linux"
        "aarch64-linux"
        "x86_64-darwin"
        "aarch64-darwin"
      ];
      forAllSystems = f: nixpkgs.lib.genAttrs systems (system: f nixpkgs.legacyPackages.${system});
    in
    {
      packages = forAllSystems (pkgs: rec {
        default = mcli;

        mcli = pkgs.buildGoModule {
          pname = "mcli";
          inherit version;
          src = ./.;

          # Update with `nix build` — it prints the expected hash on mismatch —
          # whenever go.mod or go.sum changes.
          vendorHash = "sha256-XBewtzyN7HaNA04X89p0KTBhlHE5GZ7cjMMcRhbJxtU=";

          # No cgo on any platform: the darwin keychain backend shells out to
          # /usr/bin/security rather than linking Security.framework.
          env.CGO_ENABLED = "0";

          ldflags = [
            "-s"
            "-w"
            "-X github.com/mondaycom/mcli/internal/cli.version=${version}"
          ];

          meta = {
            description = "Command-line interface for monday.com's GraphQL API, built for LLM agents";
            homepage = "https://github.com/mondaycom/mcli";
            license = nixpkgs.lib.licenses.mit;
            mainProgram = "mcli";
          };
        };
      });

      devShells = forAllSystems (pkgs: {
        default = pkgs.mkShell {
          packages = [
            pkgs.go
            pkgs.golangci-lint
            pkgs.gopls
            pkgs.gh
          ];
        };
      });
    };
}
