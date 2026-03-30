{
  description = "simple-filestore — a simple self-hosted file sharing web app";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    flake-utils.url = "github:numtide/flake-utils";
  };

  outputs = { self, nixpkgs, flake-utils }:
    flake-utils.lib.eachDefaultSystem (system:
      let pkgs = nixpkgs.legacyPackages.${system}; in {
        packages.default = pkgs.buildGoModule {
          pname = "simple-filestore";
          version = "0.1.0";
          src = ./.;
          vendorHash = null; # uses vendor/ directory

          nativeBuildInputs = [ pkgs.tailwindcss ];
          preBuild = ''
            tailwindcss -i ./input.css -o ./internal/assets/static/css/output.css --minify
          '';

          meta = {
            description = "Simple self-hosted file sharing web app";
            mainProgram = "server";
          };
        };

        devShells.default = pkgs.mkShell {
          buildInputs = [ pkgs.go pkgs.tailwindcss pkgs.air pkgs.gotools ];
          shellHook = ''
            echo "simple-filestore dev shell"
            echo "  make dev  — start with hot reload"
            echo "  make css  — start tailwind watcher"
            echo "  make test — run tests"
          '';
        };
      }
    );
}
