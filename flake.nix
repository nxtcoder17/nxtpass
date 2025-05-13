{
  description = "development workspace";

  inputs = {
    nixpkgs.url = "github:nixos/nixpkgs/nixos-unstable";
  };

  outputs = { self, nixpkgs, flake-utils }:
    flake-utils.lib.eachDefaultSystem (system:
      let
        pkgs = import nixpkgs {
          inherit system;
          # config.allowUnfree = true;
        };
      in
      {
        devShells.default = pkgs.mkShell {
          # hardeningDisable = [ "all" ];

          buildInputs = with pkgs; [
            (stdenv.mkDerivation rec {
              name = "run";
              pname = "run";
              src = fetchurl {
                url = "https://github.com/nxtcoder17/Runfile/releases/download/v1.5.1/run-linux-amd64";
                sha256 = "sha256-eR/j8+nqoo0khCnBaZg+kqNgnWRTFQDJ7jkRQuo/9Hs=";
              };
              unpackPhase = ":";
              installPhase = ''
                mkdir -p $out/bin
                cp $src $out/bin/$name
                chmod +x $out/bin/$name
              '';
            })

            # your packages here
            go
            sqlc
            sqlite
            litecli # better sqlite cli

            deno
          ];

          shellHook = ''
            export HTTP_CLI_ENV="$PWD/server/.secrets/http-cli-env.yml"
          '';
        };
      }
    );
}
