FROM golang:1.23-bookworm AS gobuild
WORKDIR /src
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/igdecoder ./cmd/igdecoder

FROM debian:bookworm-slim AS whisper
RUN apt-get update && apt-get install -y --no-install-recommends \
        git build-essential cmake wget ca-certificates clang \
    && rm -rf /var/lib/apt/lists/*
WORKDIR /w
RUN git clone --depth 1 https://github.com/ggml-org/whisper.cpp .
RUN cmake -B build \
        -DCMAKE_BUILD_TYPE=Release \
        -DBUILD_SHARED_LIBS=OFF \
        -DGGML_NATIVE=OFF \
        -DWHISPER_BUILD_TESTS=OFF \
        -DWHISPER_BUILD_EXAMPLES=ON \
        -DCMAKE_C_COMPILER=clang \
        -DCMAKE_CXX_COMPILER=clang++ \
    && cmake --build build -j --config Release
ARG WHISPER_MODEL=large-v3-turbo-q5_0
RUN bash ./models/download-ggml-model.sh ${WHISPER_MODEL}

FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends \
        ffmpeg ca-certificates \
    && rm -rf /var/lib/apt/lists/*
ARG WHISPER_MODEL=large-v3-turbo-q5_0
COPY --from=gobuild /out/igdecoder /usr/local/bin/igdecoder
COPY --from=whisper /w/build/bin/whisper-cli /usr/local/bin/whisper-cli
COPY --from=whisper /w/models/ggml-*.bin /models/
ENV IGDEC_WHISPER_MODEL=/models/ggml-${WHISPER_MODEL}.bin
ENV IGDEC_WHISPER_BIN=/usr/local/bin/whisper-cli
ENTRYPOINT ["igdecoder"]
