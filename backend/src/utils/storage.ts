import { S3Client, DeleteObjectCommand } from '@aws-sdk/client-s3'
import { getSignedUrl } from '@aws-sdk/s3-request-presigner'
import { PutObjectCommand, GetObjectCommand } from '@aws-sdk/client-s3'

export interface StorageConfig {
  accountId: string
  accessKeyId: string
  secretAccessKey: string
  bucket: string
}

function getClient(config: StorageConfig): S3Client {
  return new S3Client({
    endpoint: `https://${config.accountId}.r2.cloudflarestorage.com`,
    credentials: {
      accessKeyId: config.accessKeyId,
      secretAccessKey: config.secretAccessKey,
    },
    region: 'auto',
  })
}

export async function generatePresignedUploadUrl(
  fileKey: string,
  contentType: string,
  config: StorageConfig,
  expiresIn = 900,
): Promise<string | null> {
  if (!config.accessKeyId) return null
  try {
    const client = getClient(config)
    return await getSignedUrl(
      client,
      new PutObjectCommand({ Bucket: config.bucket, Key: fileKey, ContentType: contentType }),
      { expiresIn },
    )
  } catch {
    return null
  }
}

export async function generatePresignedGetUrl(
  fileKey: string,
  config: StorageConfig,
  expiresIn = 3600,
): Promise<string | null> {
  if (!config.accessKeyId) return null
  try {
    const client = getClient(config)
    return await getSignedUrl(
      client,
      new GetObjectCommand({ Bucket: config.bucket, Key: fileKey }),
      { expiresIn },
    )
  } catch {
    return null
  }
}

export async function deleteObject(fileKey: string, config: StorageConfig): Promise<void> {
  if (!config.accessKeyId) return
  try {
    const client = getClient(config)
    await client.send(new DeleteObjectCommand({ Bucket: config.bucket, Key: fileKey }))
  } catch {
    // best-effort
  }
}
