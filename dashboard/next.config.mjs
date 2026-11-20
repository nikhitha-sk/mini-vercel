/** @type {import('next').NextConfig} */
const nextConfig = {
  // Static export: the dashboard is plain files, so the platform can host itself.
  output: "export",
  images: { unoptimized: true },
  reactStrictMode: true,
};

export default nextConfig;
