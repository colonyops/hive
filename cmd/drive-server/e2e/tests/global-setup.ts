import { seedAll } from './harness'

export default async function globalSetup(): Promise<void> {
  await seedAll()
}
