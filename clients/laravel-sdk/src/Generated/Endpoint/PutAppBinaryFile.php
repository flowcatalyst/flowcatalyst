<?php

namespace FlowCatalyst\Generated\Endpoint;

class PutAppBinaryFile extends \FlowCatalyst\Generated\Runtime\Client\BaseEndpoint implements \FlowCatalyst\Generated\Runtime\Client\Endpoint
{
    protected $applicationCode;
    protected $sha256;
    /**
     * The body is streamed to disk, its SHA-256 checked against the address, and the file parsed: it must be a well-formed APK signed with APK Signature Scheme v2 or v3. What it says about itself (package, version, runtime version, signing certificate) is recorded and is what a publish uses. Idempotent.
     * @param string $applicationCode
     * @param string $sha256 Lower-case hex SHA-256 of the body
     * @param null|string|resource|\Psr\Http\Message\StreamInterface $requestBody
     */
    public function __construct(string $applicationCode, string $sha256, $requestBody = null)
    {
        $this->applicationCode = $applicationCode;
        $this->sha256 = $sha256;
        $this->body = $requestBody;
    }
    use \FlowCatalyst\Generated\Runtime\Client\EndpointTrait;
    public function getMethod(): string
    {
        return 'PUT';
    }
    public function getUri(): string
    {
        return str_replace(['{applicationCode}', '{sha256}'], [rawurlencode($this->applicationCode), rawurlencode($this->sha256)], '/api/app-updates/binary-files/{applicationCode}/{sha256}');
    }
    public function getBody(\Symfony\Component\Serializer\SerializerInterface $serializer, $streamFactory = null): array
    {
        if (is_string($this->body) or is_resource($this->body) or $this->body instanceof \Psr\Http\Message\StreamInterface) {
            return [['Content-Type' => ['application/octet-stream']], $this->body];
        }
        return [[], null];
    }
    public function getExtraHeaders(): array
    {
        return ['Accept' => ['application/json']];
    }
    /**
     * {@inheritdoc}
     *
     *
     * @return null|\FlowCatalyst\Generated\Model\AppBinaryFileResponse|\FlowCatalyst\Generated\Model\ErrorModel
     */
    protected function transformResponseBody(\Psr\Http\Message\ResponseInterface $response, \Symfony\Component\Serializer\SerializerInterface $serializer, ?string $contentType = null)
    {
        $status = $response->getStatusCode();
        $body = (string) $response->getBody();
        if (is_null($contentType) === false && (200 === $status && stripos(strtolower($contentType), 'application/json') !== false)) {
            return $serializer->deserialize($body, 'FlowCatalyst\Generated\Model\AppBinaryFileResponse', 'json');
        }
        if (stripos(strtolower($contentType), 'application/json') !== false) {
            return $serializer->deserialize($body, 'FlowCatalyst\Generated\Model\ErrorModel', 'json');
        }
    }
    public function getAuthenticationScopes(): array
    {
        return [];
    }
}