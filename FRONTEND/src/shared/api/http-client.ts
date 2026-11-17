export class ApiError extends Error {
  constructor(
    message: string,
    public readonly status: number,
  ) {
    super(message);
    this.name = "ApiError";
  }
}
export interface HttpClient {
  request<T>(path: string, options?: RequestInit): Promise<T>;
}
export class FetchHttpClient implements HttpClient {
  constructor(private readonly baseUrl = "/api/platform") {}
  async request<T>(path: string, options: RequestInit = {}): Promise<T> {
    const response = await fetch(`${this.baseUrl}/${path}`, {
      ...options,
      cache: "no-store",
      credentials: "same-origin",
      headers: { "Content-Type": "application/json", ...options.headers },
    });
    const envelope = await response.json().catch(() => null);
    if (!response.ok) {
      if (response.status === 401 && !path.startsWith("auth/"))
        window.dispatchEvent(new Event("session-expired"));
      throw new ApiError(
        envelope?.message || "Không thể xử lý yêu cầu. Vui lòng thử lại.",
        response.status,
      );
    }
    return envelope.data as T;
  }
}
export const errorMessage = (error: unknown) =>
  error instanceof Error ? error.message : "Đã có lỗi xảy ra.";
