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
          buildInputs = [
            pkgs.go
            pkgs.tailwindcss
            pkgs.air
            pkgs.gotools
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
      # ── App service module ────────────────────────────────────────────────
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
              description = "Path to workspace directory (config.json and folders live here)";
            };

            user = lib.mkOption {
              type = lib.types.str;
              default = "simple-filestore";
            };

            group = lib.mkOption {
              type = lib.types.str;
              default = "simple-filestore";
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
                NoNewPrivileges = true;
                ProtectSystem = "strict";
                ReadWritePaths = [ cfg.workspaceDir ];
                PrivateTmp = true;
              };
            };
          };
        };

      # ── GitHub Actions runner module ──────────────────────────────────────
      # Adds a self-hosted runner that can deploy by running nixos-rebuild.
      # Include this in your host config alongside nixosModules.default.
      nixosModules.runner = { config, lib, pkgs, ... }:
        let
          cfg = config.services.simple-filestore-runner;
          # NixOS names the runner system user "github-runner-<instance-name>"
          runnerUser = "github-runner-deploy";
        in {
          options.services.simple-filestore-runner = {
            enable = lib.mkEnableOption "GitHub Actions runner for simple-filestore deployments";

            url = lib.mkOption {
              type = lib.types.str;
              example = "https://github.com/youruser/simple-filestore";
              description = "GitHub repository URL";
            };

            tokenFile = lib.mkOption {
              type = lib.types.path;
              description = ''
                Path to a file containing the runner registration token.
                Generate one at: Settings → Actions → Runners → New self-hosted runner.
                Store it somewhere like /run/secrets/github-runner-token (not in the repo).
              '';
            };
          };

          config = lib.mkIf cfg.enable {
            services.github-runners.deploy = {
              enable = true;
              url = cfg.url;
              tokenFile = cfg.tokenFile;
              extraLabels = [ "deploy" ];
            };

            # Allow the runner user to rebuild the system — this is what
            # triggers a deployment when the workflow runs nixos-rebuild switch.
            security.sudo.extraRules = [{
              users = [ runnerUser ];
              commands = [{
                command = "${pkgs.nixos-rebuild}/bin/nixos-rebuild";
                options = [ "NOPASSWD" ];
              }];
            }];
          };
        };

      # ── Host configuration template ───────────────────────────────────────
      # Rename "filestore" to your actual LXC hostname, then reference this
      # in your system flake or use it directly with:
      #   nixos-rebuild switch --flake github:youruser/simple-filestore#filestore
      nixosConfigurations.filestore = nixpkgs.lib.nixosSystem {
        system = "x86_64-linux";
        modules = [
          self.nixosModules.default
          self.nixosModules.runner
          ({ pkgs, ... }: {
            networking.hostName = "filestore";

            # Minimal Proxmox LXC guest config
            boot.isContainer = true;
            networking.useDHCP = true;
            time.timeZone = "UTC";

            services.simple-filestore = {
              enable = true;
              port = 8080;
              # workspaceDir defaults to /var/lib/simple-filestore
            };

            services.simple-filestore-runner = {
              enable = true;
              url = "https://github.com/youruser/simple-filestore"; # ← change this
              tokenFile = "/run/secrets/github-runner-token";        # ← provision this
            };

            environment.systemPackages = [ pkgs.git pkgs.nix ];

            nix.settings.experimental-features = [ "nix-command" "flakes" ];

            system.stateVersion = "24.11";
          })
        ];
      };
    };
}
