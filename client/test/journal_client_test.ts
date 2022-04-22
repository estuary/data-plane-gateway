import { assertEquals } from "https://deno.land/std@0.135.0/testing/asserts.ts";
// TODO: Built-in snapshot testing has landed in deno std, but is not yet released.
import { test as snapshotTest } from "https://deno.land/x/snap/mod.ts";

import * as broker from "../src/gen/broker/protocol/broker.ts";
import { BASE_URL } from "./test_support.ts";
import { JournalClient, parseJournalDocuments } from "../src/journal_client.ts";
import { JournalSelector } from "../src/selector.ts";
import { readStreamToEnd } from "../src/streams.ts";

Deno.test("JournalClient.list happy path test", async () => {
  const client = new JournalClient(BASE_URL);
  const expectedJournals = [
    "acmeCo/greetings/pivot=00",
    "ops/acmeCo/logs/kind=capture/name=acmeCo%2Fsource-hello-world/pivot=00",
    "ops/acmeCo/stats/kind=capture/name=acmeCo%2Fsource-hello-world/pivot=00",
    "recovery/capture/acmeCo/source-hello-world/00000000-00000000",
  ];

  const emptySelector = new JournalSelector();
  const journals = (await client.list(emptySelector)).unwrap();

  assertEquals(4, journals.length);
  assertEquals(expectedJournals, journals.map((j) => j.name).sort());
});

Deno.test("JournalClient.list collection selector test", async () => {
  const client = new JournalClient(BASE_URL);

  const name = new JournalSelector().collection("acmeCo/greetings");
  const journals = (await client.list(name)).unwrap();

  assertEquals(1, journals.length);
  assertEquals("acmeCo/greetings/pivot=00", journals[0].name);
});

Deno.test("JournalClient.list name selector test", async () => {
  const client = new JournalClient(BASE_URL);

  const name = new JournalSelector().name("acmeCo/greetings/pivot=00");
  const journals = (await client.list(name)).unwrap();

  assertEquals(1, journals.length);
  assertEquals("acmeCo/greetings/pivot=00", journals[0].name);
});

Deno.test("JournalClient.list prefix selector test", async () => {
  const client = new JournalClient(BASE_URL);
  const expectedJournals = [
    "ops/acmeCo/logs/kind=capture/name=acmeCo%2Fsource-hello-world/pivot=00",
    "ops/acmeCo/stats/kind=capture/name=acmeCo%2Fsource-hello-world/pivot=00",
  ];

  const prefixSelector = JournalSelector.prefix("ops");
  const journals = (await client.list(prefixSelector)).unwrap();

  assertEquals(2, journals.length);
  assertEquals(expectedJournals, journals.map((j) => j.name).sort());
});

Deno.test("JournalClient.list exclusion selector test", async () => {
  const client = new JournalClient(BASE_URL);

  const prefixSelector = JournalSelector.prefix("ops");
  const excludedSelector = new JournalSelector().collection("ops/acmeCo/stats");
  const journals = (await client.list(prefixSelector, excludedSelector))
    .unwrap();

  assertEquals(1, journals.length);
  assertEquals(
    "ops/acmeCo/logs/kind=capture/name=acmeCo%2Fsource-hello-world/pivot=00",
    journals[0].name,
  );
});

snapshotTest("JournalClient.read test", async ({ assertSnapshot }) => {
  const client = new JournalClient(BASE_URL);

  const req = { journal: "acmeCo/greetings/pivot=00", endOffset: "1024" };
  const stream = (await client.read(req)).unwrap();
  const results: Array<broker.ProtocolReadResponse> = await readStreamToEnd(
    stream,
  );

  // Pluck out a few interesting properties to snapshot on. Many of the
  // properties are not stable between clusters, so snapshotting the entire
  // response would involve redacting a lot of fields.
  const pluck = (res: broker.ProtocolReadResponse) => {
    return {
      status: res.status,
      fragment: {
        journal: res!.fragment?.journal,
        compressionCodec: res!.fragment?.compressionCodec,
      },
    };
  };

  assertSnapshot(results.map(pluck));
});

snapshotTest("JournalClient.read content test", async ({ assertSnapshot }) => {
  const client = new JournalClient(BASE_URL);

  const req = {
    journal: "acmeCo/greetings/pivot=00",
    offset: "10",
    endOffset: "1024",
  };
  const stream = (await client.read(req)).unwrap();
  const docStream = parseJournalDocuments(stream!);
  const results = await readStreamToEnd(docStream);

  const masks = ["/*/_meta/uuid"];
  assertSnapshot(results, masks);
});
