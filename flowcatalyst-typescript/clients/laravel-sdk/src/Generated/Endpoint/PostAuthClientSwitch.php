<?php

namespace FlowCatalyst\Generated\Endpoint;

class PostAuthClientSwitch extends \FlowCatalyst\Generated\Runtime\Client\BaseEndpoint implements \FlowCatalyst\Generated\Runtime\Client\Endpoint
{
    /**
     * @param null|\FlowCatalyst\Generated\Model\SwitchClientRequest $requestBody
     */
    public function __construct(?\FlowCatalyst\Generated\Model\SwitchClientRequest $requestBody = null)
    {
        $this->body = $requestBody;
    }
    use \FlowCatalyst\Generated\Runtime\Client\EndpointTrait;
    public function getMethod(): string
    {
        return 'POST';
    }
    public function getUri(): string
    {
        return '/auth/client/switch';
    }
    public function getBody(\Symfony\Component\Serializer\SerializerInterface $serializer, $streamFactory = null): array
    {
        if ($this->body instanceof \FlowCatalyst\Generated\Model\SwitchClientRequest) {
            return [['Content-Type' => ['application/json']], $serializer->serialize($this->body, 'json')];
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
     * @throws \FlowCatalyst\Generated\Exception\PostAuthClientSwitchUnauthorizedException
     * @throws \FlowCatalyst\Generated\Exception\PostAuthClientSwitchForbiddenException
     * @throws \FlowCatalyst\Generated\Exception\PostAuthClientSwitchBadRequestException
     *
     * @return null|\FlowCatalyst\Generated\Model\SwitchClientResponse
     */
    protected function transformResponseBody(\Psr\Http\Message\ResponseInterface $response, \Symfony\Component\Serializer\SerializerInterface $serializer, ?string $contentType = null)
    {
        $status = $response->getStatusCode();
        $body = (string) $response->getBody();
        if (is_null($contentType) === false && (200 === $status && mb_strpos(strtolower($contentType), 'application/json') !== false)) {
            return $serializer->deserialize($body, 'FlowCatalyst\Generated\Model\SwitchClientResponse', 'json');
        }
        if (401 === $status) {
            throw new \FlowCatalyst\Generated\Exception\PostAuthClientSwitchUnauthorizedException($response);
        }
        if (403 === $status) {
            throw new \FlowCatalyst\Generated\Exception\PostAuthClientSwitchForbiddenException($response);
        }
        if (400 === $status) {
            throw new \FlowCatalyst\Generated\Exception\PostAuthClientSwitchBadRequestException($response);
        }
    }
    public function getAuthenticationScopes(): array
    {
        return [];
    }
}