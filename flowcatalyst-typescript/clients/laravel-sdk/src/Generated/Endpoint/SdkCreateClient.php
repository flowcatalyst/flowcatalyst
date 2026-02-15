<?php

namespace FlowCatalyst\Generated\Endpoint;

class SdkCreateClient extends \FlowCatalyst\Generated\Runtime\Client\BaseEndpoint implements \FlowCatalyst\Generated\Runtime\Client\Endpoint
{
    /**
     * @param null|\FlowCatalyst\Generated\Model\CreateClientRequest1 $requestBody
     */
    public function __construct(?\FlowCatalyst\Generated\Model\CreateClientRequest1 $requestBody = null)
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
        return '/api/sdk/clients';
    }
    public function getBody(\Symfony\Component\Serializer\SerializerInterface $serializer, $streamFactory = null): array
    {
        if ($this->body instanceof \FlowCatalyst\Generated\Model\CreateClientRequest1) {
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
     * @throws \FlowCatalyst\Generated\Exception\SdkCreateClientBadRequestException
     * @throws \FlowCatalyst\Generated\Exception\SdkCreateClientUnauthorizedException
     * @throws \FlowCatalyst\Generated\Exception\SdkCreateClientForbiddenException
     *
     * @return null|\FlowCatalyst\Generated\Model\ClientDto1
     */
    protected function transformResponseBody(\Psr\Http\Message\ResponseInterface $response, \Symfony\Component\Serializer\SerializerInterface $serializer, ?string $contentType = null)
    {
        $status = $response->getStatusCode();
        $body = (string) $response->getBody();
        if (is_null($contentType) === false && (201 === $status && mb_strpos(strtolower($contentType), 'application/json') !== false)) {
            return $serializer->deserialize($body, 'FlowCatalyst\Generated\Model\ClientDto1', 'json');
        }
        if (400 === $status) {
            throw new \FlowCatalyst\Generated\Exception\SdkCreateClientBadRequestException($response);
        }
        if (401 === $status) {
            throw new \FlowCatalyst\Generated\Exception\SdkCreateClientUnauthorizedException($response);
        }
        if (403 === $status) {
            throw new \FlowCatalyst\Generated\Exception\SdkCreateClientForbiddenException($response);
        }
    }
    public function getAuthenticationScopes(): array
    {
        return [];
    }
}