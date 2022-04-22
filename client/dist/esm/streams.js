export function splitStream(splitOn) {
    let buffer = "";
    function queueLines(controller) {
        const parts = buffer.split(splitOn);
        parts.slice(0, -1).forEach((part) => controller.enqueue(part));
        buffer = parts[parts.length - 1];
    }
    return new TransformStream({
        // Upon receiving another chunk through the stream, send each line along as
        // its own chunk.
        transform(chunk, controller) {
            buffer += chunk;
            queueLines(controller);
        },
        // Upon closing the stream, try once more to split any lines. Any incomplete
        // lines are dropped. This avoids consumers needing to deal with incomplete
        // records.
        flush(controller) {
            if (buffer.length) {
                queueLines(controller);
            }
        },
    });
}
export function parseJSONStream() {
    return new TransformStream({
        transform(chunk, controller) {
            try {
                controller.enqueue(JSON.parse(chunk));
            }
            catch (_e) {
                // We failed to parse this chunk as json. Skip it. Incomplete records
                // are expected (we don't always know which offset to start reading
                // from) and are not fatal.
            }
        },
    });
}
export function unwrapResult() {
    return new TransformStream({
        transform(chunk, controller) {
            // The GRPC Gateway wraps all the reads in a `result` field that we don't
            // want to deal with.
            controller.enqueue(chunk.result);
        },
    });
}
export function decodeContent() {
    return new TransformStream({
        transform(value, controller) {
            // Base64 decode the `content` field and send it as a chunk.
            if (value.content?.length) {
                controller.enqueue(atob(value.content));
            }
        },
    });
}
// Helper to fully read a stream and accumulates its values.
export async function readStreamToEnd(stream) {
    const results = [];
    const reader = stream.getReader();
    await reader.read().then(async function pump({ done, value }) {
        if (!done && value) {
            results.push(value);
            return pump(await reader.read());
        }
    });
    return results;
}
