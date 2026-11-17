import { assertEquals } from "https://deno.land/std@0.135.0/testing/asserts.ts";

import { JournalClient } from "../src/journal_client.ts";
import { JournalSelector } from "../src/selector.ts";

Deno.test("JournalClient.list merges frames across network chunks", async () => {
  const journals = Array.from({ length: 32 }, (_, index) => ({
    spec: { name: `acmeCo/greetings/journal-${index}` },
    modRevision: `${index + 1}`,
    route: { primary: 0, members: [{ zone: "test", suffix: `${index}` }] },
  }));
  const header = { etcd: { revision: "123" } };
  const firstFrame = JSON.stringify({
    result: { status: "OK", header, journals: journals.slice(0, 25) },
  });
  const body = new TextEncoder().encode(
    firstFrame + "\n" + JSON.stringify({
      result: { journals: journals.slice(25) },
    }) + "\n",
  );
  const boundaries = [
    0,
    17,
    firstFrame.length,
    firstFrame.length + 2,
    body.length,
  ];
  const originalFetch = globalThis.fetch;

  globalThis.fetch = () =>
    Promise.resolve(
      new Response(
        new ReadableStream<Uint8Array>({
          start(controller) {
            for (let index = 1; index < boundaries.length; index++) {
              controller.enqueue(
                body.slice(boundaries[index - 1], boundaries[index]),
              );
            }
            controller.close();
          },
        }),
      ),
    );

  try {
    const client = new JournalClient(
      new URL("https://example.test/"),
      "test-token",
    );
    const response = await client.list(
      new JournalSelector().collection("acmeCo/greetings"),
    );

    assertEquals(response.unwrap(), { status: "OK", header, journals });
  } finally {
    globalThis.fetch = originalFetch;
  }
});
