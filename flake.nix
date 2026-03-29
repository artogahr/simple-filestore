{
  description = "simple-filestore — a simple self-hosted file sharing web app";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    flake-utils.url = "github:numtide/flake-utils";
  };

  outputs = { self, nixpkgs, flake-utils }:
    flake-utils.lib.eachDefaultSystem (system:
      let
        pkgs = nixpkgs.legacyPackages.${system};
      in {
        packages.default = pkgs.buildGoModule {
          pname = "simple-filestore";
          version = "0.1.0";
          src = ./.;
          vendorHash = null; # set after `go mod vendor`

          nativeBuildInputs = [ pkgs.tailwindcss ];

          preBuild = ''
            tailwindcss -i ./input.css -o ./static/css/output.css --minify
          '';

          meta = {
            description = "Simple self-hosted file sharing web app";
            mainProgram = "server";
          };
        };

        devShells.default = pkgs.mkShell {
          buildInputs = [
            pkgs.go
            pkgs.tailwindcss
            pkgs.air
            pkgs.gotools  # goimports, etc.
          ];
          shellHook = ''
            echo "simple-filestore dev shell"
            echo "  make dev      — start with hot reload (requires two terminals for css)"
            echo "  make css      — start tailwind watcher"
            echo "  make test     — run tests"
            echo "  make fmt      — format code"
          '';
        };
      }
    ) // {
      nixosModules.default = { config, lib, pkgs, ... }:
        let
          cfg = config.services.simple-filestore;
          pkg = self.packages.${pkgs.system}.default;
        in {
          options.services.simple-filestore = {
            enable = lib.mkEnableOption "simple-filestore file sharing service";

            port = lib.mkOption {
              type = lib.types.port;
              default = 8080;
              description = "Port to listen on";
            };

            workspaceDir = lib.mkOption {
              type = lib.types.str;
              default = "/var/lib/simple-filestore";
              description = "Path to the workspace directory (contains config.json and folders)";
            };

            user = lib.mkOption {
              type = lib.types.str;
              default = "simple-filestore";
              description = "User to run the service as";
            };

            group = lib.mkOption {
              type = lib.types.str;
              default = "simple-filestore";
              description = "Group to run the service as";
            };
          };

          config = lib.mkIf cfg.enable {
            users.users.${cfg.user} = {
              isSystemUser = true;
              group = cfg.group;
              home = cfg.workspaceDir;
              createHome = false;
            };

            users.groups.${cfg.group} = {};

            systemd.services.simple-filestore = {
              description = "simple-filestore file sharing service";
              wantedBy = [ "multi-user.target" ];
              after = [ "network.target" ];

              serviceConfig = {
                ExecStart = "${pkg}/bin/server --workspace ${cfg.workspaceDir} --port ${toString cfg.port}";
                User = cfg.user;
                Group = cfg.group;
                Restart = "on-failure";
                StateDirectory = "simple-filestore";
                StateDirectoryMode = "0750";
                # Security hardening
                NoNewPrivileges = true;
                ProtectSystem = "strict";
                ReadWritePaths = [ cfg.workspaceDir ];
                PrivateTmp = true;
              };
            };
          };
        };
    };
}
