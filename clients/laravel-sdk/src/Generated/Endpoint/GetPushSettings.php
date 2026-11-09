<?php

namespace FlowCatalyst\Generated\Endpoint;

class GetPushSettings extends \FlowCatalyst\Generated\Runtime\Client\BaseEndpoint implements \FlowCatalyst\Generated\Runtime\Client\Endpoint
{
    protected $appCode;
    /**
     * @param string $appCode Application code
     */
    public function __construct(string $appCode)
    {
        $this->appCode = $appCode;
    }
    use \FlowCatalyst\Generated\Runtime\Client\EndpointTrait;
    public function getMethod(): string
    {
        return 'GET';
    }
    public function getUri(): string
    {
        return str_replace(['{appCode}'], [rawurlencode($this->appCode)], '/api/applications/{appCode}/push/settings');
    }
    public function getBody(\Symfony\Component\Serializer\SerializerInterface $serializer, $streamFactory = null): array
    {
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
     * @return null|\FlowCatalyst\Generated\Model\PushSettingsResponse|\FlowCatalyst\Generated\Model\ErrorModel
     */
    protected function transformResponseBody(\Psr\Http\Message\ResponseInterface $response, \Symfony\Component\Serializer\SerializerInterface $serializer, ?string $contentType = null)
    {
        $status = $response->getStatusCode();
        $body = (string) $response->getBody();
        if (is_null($contentType) === false && (200 === $status && stripos(strtolower($contentType), 'application/json') !== false)) {
            return $serializer->deserialize($body, 'FlowCatalyst\Generated\Model\PushSettingsResponse', 'json');
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