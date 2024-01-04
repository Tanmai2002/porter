import { z } from "zod";

export const datastoreEnvValidator = z.object({
  name: z.string(),
  linked_applications: z.string().array().default([]),
  secret_variables: z.record(z.string()).default({}),
  variables: z.record(z.string()).default({}),
  version: z.number(),
});

export type DatastoreEnvWithSource = z.infer<typeof datastoreEnvValidator>;

export const datastoreMetadataValidator = z.object({
  name: z.string(),
  value: z.string().default(""),
});

export type DatastoreMetadataWithSource = z.infer<
  typeof datastoreMetadataValidator
>;

export const datastoreValidator = z.object({
  name: z.string(),
  type: z.string(),
  status: z.string().default(""),
  metadata: datastoreMetadataValidator.array().default([]),
  env: datastoreEnvValidator.optional(),
  connection_string: z.string().default(""),
});

export type DatastoreWithSource = z.infer<typeof datastoreValidator>;

export const datastoreListResponseValidator = z.object({
  datastores: datastoreValidator.array(),
});

export const cloudProviderValidator = z.object({
  cloud_provider_id: z.string(),
  project_id: z.number(),
});

export type CloudProviderWithSource = z.infer<typeof cloudProviderValidator>;

export const cloudProviderListResponseValidator = z.object({
  accounts: cloudProviderValidator.array(),
});

export const cloudProviderDatastoreSchema = z.object({
  project_id: z.number(),
  cloud_provider_name: z.string(),
  cloud_provider_id: z.string(),
  datastore: datastoreValidator,
});

export type CloudProviderDatastore = z.infer<
  typeof cloudProviderDatastoreSchema
>;