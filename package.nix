{
  lib,
  buildGoModule,
  makeWrapper,
  yt-dlp,
  version ? "dev",
  src ? lib.cleanSource ./.,
}:

buildGoModule {
  pname = "ytdlpick";
  inherit version src;

  vendorHash = "sha256-QdyPsuD8TMk51Zq3jXa5c7pllNGzHoCPkYHzQx0AepA=";

  ldflags = [
    "-s"
    "-w"
  ];

  nativeBuildInputs = [ makeWrapper ];

  # yt-dlp (which itself brings ffmpeg) is looked up on PATH unless
  # -ytdlp or $YTDLP says otherwise.
  postInstall = ''
    wrapProgram $out/bin/ytdlpick \
      --prefix PATH : ${lib.makeBinPath [ yt-dlp ]}
  '';

  meta = {
    description = "Interactive TUI to pick video, audio and subtitle tracks for yt-dlp";
    homepage = "https://github.com/rck/ytdlpick";
    license = lib.licenses.gpl3Plus;
    mainProgram = "ytdlpick";
    platforms = lib.platforms.unix;
  };
}
