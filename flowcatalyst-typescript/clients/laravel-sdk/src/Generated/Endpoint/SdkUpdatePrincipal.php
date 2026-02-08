<?php

namespace FlowCatalyst\Generated\Endpoint;

class SdkUpdatePrincipal extends \FlowCatalyst\Generated\Runtime\Client\BaseEndpoint implements \FlowCatalyst\Generated\Runtime\Client\Endpoint
{
    protected $id;
    /**
     * @param string $id
     * @param null|\FlowCatalyst\Generated\Model\UpdatePrincipalRequest $requestBody
     */
    public function __construct(string $id, ?\FlowCatalyst\Generated\Model\UpdatePrincipalRequest $requestBody = null)
    {
        $this->id = $id;
        $this->body = $requestBody;
    }
    use \FlowCatalyst\Generated\Runtime\Client\EndpointTrait;
    public function getMethod(): string
    {
        return 'PUT';
    }
    public function getUri(): string
    {
        return str_replace(['{id}'], [$this->id], '/api/sdk/principals/{id}');
    }
    public function getBody(\Symfony\Component\Serializer\SerializerInterface $serializer, $streamFactory = null): array
    {
        if ($this->body instanceof \FlowCatalyst\Generated\Model\UpdatePrincipalRequest) {
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
     * @throws \FlowCatalyst\Generated\Exception\SdkUpdatePrincipalNotFoundException
     * @throws \FlowCatalyst\Generated\Exception\SdkUpdatePrincipalUnauthorizedException
     * @throws \FlowCatalyst\Generated\Exception\SdkUpdatePrincipalForbiddenException
     * @throws \FlowCatalyst\Generated\Exception\SdkUpdatePrincipalBadRequestException
     *
     * @return null|\FlowCatalyst\Generated\Model\PrincipalDto
     */
    protected function transformResponseBody(\Psr\Http\Message\ResponseInterface $response, \Symfony\Component\Serializer\SerializerInterface $serializer, ?string $contentType = null)
    {
        $status = $response->getStatusCode();
        $body = (string) $response->getBody();
        if (is_null($contentType) === false && (200 === $status && mb_strpos(strtolower($contentType), 'application/json') !== false)) {
            return $serializer->deserialize($body, 'FlowCatalyst\Generated\Model\PrincipalDto', 'json');
        }
        if (404 === $status) {
            throw new \FlowCatalyst\Generated\Exception\SdkUpdatePrincipalNotFoundException($response);
        }
        if (401 === $status) {
            throw new \FlowCatalyst\Generated\Exception\SdkUpdatePrincipalUnauthorizedException($response);
        }
        if (403 === $status) {
            throw new \FlowCatalyst\Generated\Exception\SdkUpdatePrincipalForbiddenException($response);
        }
        if (400 === $status) {
            throw new \FlowCatalyst\Generated\Exception\SdkUpdatePrincipalBadRequestException($response);
        }
    }
    public function getAuthenticationScopes(): array
    {
        return [];
    }
}