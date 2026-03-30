{
  description = "simple-filestore — a simple self-hosted file sharing web app";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    flake-utils.url = "github:numtide/flake-utils";
    nixos-generators = {
      url = "github:nix-community/nixos-generators";
      inputs.nixpkgs.follows = "nixpkgs";
    };
  };

  outputs = { self, nixpkgs, flake-utils, nixos-generators }:
    let
      # Per-system outputs (app package + devShell)
      perSystem = flake-utils.lib.eachDefaultSystem (system:
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
              echo "  make dev      — start with hot reload (requires two terminals for css)"
              echo "  make css      — start tailwind watcher"
              echo "  make test     — run tests"
              echo "  make fmt      — format code"
            '';
          };
        }
      );
    in
    perSystem // {
      # Merge lxc-template into the existing x86_64-linux packages without
      # overwriting the default package that eachDefaultSystem already produced.
      packages = perSystem.packages // {
        x86_64-linux = perSystem.packages.x86_64-linux // {
          # Proxmox LXC bootstrap image.
          # Build:  nix build .#lxc-template
          # Upload: scp result root@proxmox:/var/lib/vz/template/cache/nixos-simple-filestore.tar.xz
          # Then create an LXC from that template in the Proxmox UI.
          lxc-template = nixos-generators.nixosGenerate {
            system = "x86_64-linux";
            format = "proxmox-lxc";
            modules = [{
              networking.hostName = "simple-filestore";
              boot.isContainer = true;
              time.timeZone = "UTC";
              nix.settings.experimental-features = [ "nix-command" "flakes" ];
              services.openssh = {
                enable = true;
                settings.PermitRootLogin = "yes";
              };
              # Temporary password for first login — change after bootstrap.
              users.users.root.initialPassword = "nixos";
              system.stateVersion = "24.11";
            }];
          };
        };
      };

      # ── App service module ────────────────────────────────────────────────
      nixosModules.default = { config, lib, pkgs, ... }:
        let
          cfg = config.services.simple-filestore;
          pkg = self.packages.${pkgs.system}.default;
        in {
          options.services.simple-filestore = {
            enable = lib.mkEnableOption "simple-filestore file sharing service";
            port = lib.mkOption { type = lib.types.port; default = 8080; };
            workspaceDir = lib.mkOption {
              type = lib.types.str;
              default = "/var/lib/simple-filestore";
              description = "Path to workspace directory (config.json and folders live here)";
            };
            user  = lib.mkOption { type = lib.types.str; default = "simple-filestore"; };
            group = lib.mkOption { type = lib.types.str; default = "simple-filestore"; };
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
      # Adds a self-hosted runner that deploys by running nixos-rebuild switch.
      nixosModules.runner = { config, lib, pkgs, ... }:
        let
          cfg = config.services.simple-filestore-runner;
          runnerUser = "github-runner-deploy"; # NixOS names it github-runner-<instance>
        in {
          options.services.simple-filestore-runner = {
            enable = lib.mkEnableOption "GitHub Actions runner for simple-filestore deployments";
            url = lib.mkOption {
              type = lib.types.str;
              example = "https://github.com/youruser/simple-filestore";
            };
            tokenFile = lib.mkOption {
              type = lib.types.path;
              description = "Path to file containing the runner registration token.";
            };
          };

          config = lib.mkIf cfg.enable {
            services.github-runners.deploy = {
              enable = true;
              url = cfg.url;
              tokenFile = cfg.tokenFile;
              extraLabels = [ "deploy" ];
            };

            security.sudo.extraRules = [{
              users = [ runnerUser ];
              commands = [{
                command = "${pkgs.nixos-rebuild}/bin/nixos-rebuild";
                options = [ "NOPASSWD" ];
              }];
            }];
          };
        };

      # ── Host configuration ────────────────────────────────────────────────
      # After creating the LXC from lxc-template, run on the container:
      #   nixos-rebuild switch --flake github:youruser/simple-filestore#simple-filestore
      nixosConfigurations.simple-filestore = nixpkgs.lib.nixosSystem {
        system = "x86_64-linux";
        modules = [
          self.nixosModules.default
          self.nixosModules.runner
          ({ pkgs, ... }: {
            networking.hostName = "simple-filestore";
            boot.isContainer = true;
            networking.useDHCP = true;
            time.timeZone = "UTC";

            services.simple-filestore = {
              enable = true;
              port = 8080;
            };

            services.simple-filestore-runner = {
              enable = true;
              url = "https://github.com/youruser/simple-filestore"; # ← your repo URL
              tokenFile = "/run/secrets/github-runner-token";        # ← provision this file
            };

            environment.systemPackages = [ pkgs.git ];
            nix.settings.experimental-features = [ "nix-command" "flakes" ];
            system.stateVersion = "24.11";
          })
        ];
      };
    };
}
