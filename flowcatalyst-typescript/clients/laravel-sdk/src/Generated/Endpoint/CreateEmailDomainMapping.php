<?php

namespace FlowCatalyst\Generated\Endpoint;

class CreateEmailDomainMapping extends \FlowCatalyst\Generated\Runtime\Client\BaseEndpoint implements \FlowCatalyst\Generated\Runtime\Client\Endpoint
{
    /**
     * @param null|\FlowCatalyst\Generated\Model\CreateEmailDomainMappingRequest $requestBody
     */
    public function __construct(?\FlowCatalyst\Generated\Model\CreateEmailDomainMappingRequest $requestBody = null)
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
        return '/api/admin/email-domain-mappings';
    }
    public function getBody(\Symfony\Component\Serializer\SerializerInterface $serializer, $streamFactory = null): array
    {
        if ($this->body instanceof \FlowCatalyst\Generated\Model\CreateEmailDomainMappingRequest) {
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
     * @throws \FlowCatalyst\Generated\Exception\CreateEmailDomainMappingBadRequestException
     * @throws \FlowCatalyst\Generated\Exception\CreateEmailDomainMappingNotFoundException
     * @throws \FlowCatalyst\Generated\Exception\CreateEmailDomainMappingConflictException
     *
     * @return null|\FlowCatalyst\Generated\Model\EmailDomainMappingDto
     */
    protected function transformResponseBody(\Psr\Http\Message\ResponseInterface $response, \Symfony\Component\Serializer\SerializerInterface $serializer, ?string $contentType = null)
    {
        $status = $response->getStatusCode();
        $body = (string) $response->getBody();
        if (is_null($contentType) === false && (201 === $status && mb_strpos(strtolower($contentType), 'application/json') !== false)) {
            return $serializer->deserialize($body, 'FlowCatalyst\Generated\Model\EmailDomainMappingDto', 'json');
        }
        if (400 === $status) {
            throw new \FlowCatalyst\Generated\Exception\CreateEmailDomainMappingBadRequestException($response);
        }
        if (404 === $status) {
            throw new \FlowCatalyst\Generated\Exception\CreateEmailDomainMappingNotFoundException($response);
        }
        if (409 === $status) {
            throw new \FlowCatalyst\Generated\Exception\CreateEmailDomainMappingConflictException($response);
        }
    }
    public function getAuthenticationScopes(): array
    {
        return [];
    }
}