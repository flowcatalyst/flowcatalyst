<?php

namespace FlowCatalyst\Generated\Endpoint;

class SdkCreateServicePrincipal extends \FlowCatalyst\Generated\Runtime\Client\BaseEndpoint implements \FlowCatalyst\Generated\Runtime\Client\Endpoint
{
    /**
     * @param null|\FlowCatalyst\Generated\Model\CreateServicePrincipalRequest $requestBody
     */
    public function __construct(?\FlowCatalyst\Generated\Model\CreateServicePrincipalRequest $requestBody = null)
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
        return '/api/sdk/principals/service';
    }
    public function getBody(\Symfony\Component\Serializer\SerializerInterface $serializer, $streamFactory = null): array
    {
        if ($this->body instanceof \FlowCatalyst\Generated\Model\CreateServicePrincipalRequest) {
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
     * @throws \FlowCatalyst\Generated\Exception\SdkCreateServicePrincipalBadRequestException
     * @throws \FlowCatalyst\Generated\Exception\SdkCreateServicePrincipalUnauthorizedException
     * @throws \FlowCatalyst\Generated\Exception\SdkCreateServicePrincipalForbiddenException
     *
     * @return null|\FlowCatalyst\Generated\Model\StatusResponse1
     */
    protected function transformResponseBody(\Psr\Http\Message\ResponseInterface $response, \Symfony\Component\Serializer\SerializerInterface $serializer, ?string $contentType = null)
    {
        $status = $response->getStatusCode();
        $body = (string) $response->getBody();
        if (is_null($contentType) === false && (201 === $status && mb_strpos(strtolower($contentType), 'application/json') !== false)) {
            return $serializer->deserialize($body, 'FlowCatalyst\Generated\Model\StatusResponse1', 'json');
        }
        if (400 === $status) {
            throw new \FlowCatalyst\Generated\Exception\SdkCreateServicePrincipalBadRequestException($response);
        }
        if (401 === $status) {
            throw new \FlowCatalyst\Generated\Exception\SdkCreateServicePrincipalUnauthorizedException($response);
        }
        if (403 === $status) {
            throw new \FlowCatalyst\Generated\Exception\SdkCreateServicePrincipalForbiddenException($response);
        }
    }
    public function getAuthenticationScopes(): array
    {
        return [];
    }
}