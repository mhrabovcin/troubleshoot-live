{
  description = "Run troubleshoot-live with nix";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
  };

  outputs = { self, nixpkgs }:
    let
      systems = nixpkgs.lib.systems.flakeExposed;
      forAllSystems = f: nixpkgs.lib.genAttrs systems (system: f system);
    in
    {
      packages = forAllSystems (system:
        let
          pkgs = import nixpkgs { inherit system; };
        in
        {
          troubleshoot-live = pkgs.buildGoModule {
            pname = "troubleshoot-live";
            version = "0.0.0";
            src = self;

            subPackages = [ "." ];

            # Replace with real hash from nix build output.
            vendorHash = "sha256-TLKUDh9Ndg4cBWPU9lWgz1VNV1kBYgEVk/0RdYP8eSw=";

            ldflags = [
              "-s"
              "-w"
            ];

            meta = with pkgs.lib; {
              description = "Expose support bundles as a local Kubernetes API server";
              homepage = "https://github.com/mhrabovcin/troubleshoot-live";
              license = licenses.asl20;
              mainProgram = "troubleshoot-live";
            };
          };

          default = self.packages.${system}.troubleshoot-live;
        });

      apps = forAllSystems (system: {
        default = {
          type = "app";
          program = "${self.packages.${system}.default}/bin/troubleshoot-live";
        };
      });
    };
}
